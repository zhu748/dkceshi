package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/internal/devid"
)

// androidVersionByAPI 把 android_api_level 映射成 Android 主版本号（UA 里的
// "Android 16" 与 Chrome-on-Android 指纹的 OS 版本必须同源）。未知 level 兜底 16
// （与仓库 HAR 基准 api 36 = Android 16 同代）。
var androidVersionByAPI = map[string]int{
	"33": 13, "34": 14, "35": 15, "36": 16, "37": 17,
}

// tzNameByOffset 把 x-client-timezone-offset（秒）映射成 IANA 时区名。
// 覆盖 DeepSeek 实际常见地区；未列出的偏移兜底 Asia/Shanghai。
var tzNameByOffset = map[int]string{
	28800: "Asia/Shanghai",
	32400: "Asia/Tokyo",
	0:     "Etc/UTC",
}

// buildDevidAnchor 把账号声称的设备身份（x-device-model / x-client-locale /
// x-client-timezone-offset / android_api_level）汇总成 devid 生成器的安卓锚定文件。
// 生成器据此签发与 HTTP 层同一台"手机"的指纹，消除"桌面 Windows 指纹 + 安卓 App
// 身份"的矛盾。返回临时文件路径，调用方负责删除。
func buildDevidAnchor(identifier string, acc config.Account) (string, error) {
	bh := dsprotocol.BaseHeaders
	deviceModel := strings.TrimSpace(acc.DeviceModel)
	if deviceModel == "" {
		deviceModel = strings.TrimSpace(bh["x-device-model"])
	}
	if deviceModel == "" {
		return "", nil // 没有设备身份可锚，保持旧行为
	}
	locale := strings.TrimSpace(acc.Locale)
	if locale == "" {
		locale = strings.TrimSpace(bh["x-client-locale"])
	}
	// x-client-locale 是下划线格式（zh_CN），浏览器 locale 是中划线（zh-CN）
	locale = strings.ReplaceAll(locale, "_", "-")

	apiLevel := strings.TrimSpace(acc.AndroidAPILevel)
	if apiLevel == "" {
		if m := regexpUAAndroid.FindStringSubmatch(strings.TrimSpace(bh["User-Agent"])); m != nil {
			apiLevel = m[1]
		}
	}
	androidVersion := androidVersionByAPI[apiLevel]
	if androidVersion == 0 {
		androidVersion = 16
	}

	tzOffsetSec := 28800
	if v, err := strconv.Atoi(strings.TrimSpace(bh["x-client-timezone-offset"])); err == nil {
		tzOffsetSec = v
	}
	tzName := acc.Timezone
	if tzName != "" {
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			return "", err
		}
		_, tzOffsetSec = time.Now().In(loc).Zone()
	} else if name, ok := tzNameByOffset[tzOffsetSec]; ok {
		tzName = name
	} else if tzOffsetSec%3600 == 0 {
		// Etc/GMT names have the opposite sign; they carry no DST.
		tzName = fmt.Sprintf("Etc/GMT%+d", -tzOffsetSec/3600)
	} else {
		return "", fmt.Errorf("devid: timezone is required for offset %d", tzOffsetSec)
	}
	// JS getTimezoneOffset 语义：分钟，东经为负（UTC+8 → -480）
	jsTZOffset := -tzOffsetSec / 60

	anchor := map[string]any{
		"device_model":    deviceModel,
		"android_version": androidVersion,
		"locale":          locale,
		"tz":              tzName,
		"tz_offset":       jsTZOffset,
	}
	raw, err := jsonMarshalIndent(anchor)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(config.DevidDir(), "anchors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("devid: 创建锚定目录失败: %v", err)
	}
	p := filepath.Join(dir, config.SanitizeFilename(identifier)+".json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return "", fmt.Errorf("devid: 写锚定文件失败: %v", err)
	}
	return p, nil
}

// regexpUAAndroid 从 UA（DeepSeek/2.7.1 Android/36）里提 api level。
var regexpUAAndroid = regexp.MustCompile(`Android/([0-9]+)`)

func jsonMarshalIndent(v any) ([]byte, error) {
	// 小工具：锚定文件用缩进 JSON，便于人工检查。
	return json.MarshalIndent(v, "", "  ")
}

// devidMaxConcurrency 是批量刷新真实设备指纹时的并发上限。指纹生成本身要
// 起一个无头浏览器并等待数美云端签发，并发过高既慢又容易被风控关注，保持
// 保守的小并发。
const devidMaxConcurrency = 2

// devidGenTimeout 是单账号指纹生成的等待上限，与 admin HTTP 请求的容忍时长相符。
const devidGenTimeout = 150 * time.Second

// devidGenLocks 串行化同一账号的指纹生成，避免并发刷新把同一账号的画像文件
// 写穿。账号删除时由 ForgetDeviceGenLock 清理。
var devidGenLocks sync.Map // map[string]*sync.Mutex

// ForgetDeviceGenLock 清理已删除账号的指纹生成锁（实现 admin 侧可选接口）。
func (c *Client) ForgetDeviceGenLock(identifier string) {
	if c == nil || identifier == "" {
		return
	}
	devidGenLocks.Delete(identifier)
	devidGenLocks.Delete("login:" + identifier)
}

func lockDevidGen(identifier string) *sync.Mutex {
	mu, _ := devidGenLocks.LoadOrStore(identifier, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// RefreshRealDeviceID 为单个账号生成数美真实设备指纹并写回配置，实现
// POST /admin/device-ids/real/refresh 的底层能力。
//
// resetProfile 语义与后台页面对齐：
//   - false（刷新 id）：复用既有浏览器画像重新上报，设备身份不变，
//     但数美每次上报都会签发新的 deviceId；
//   - true（重置设备档案）：先删除画像再生成，等于给账号换一台新设备。
//
// 生成成功后 device_id 与 device_id_type=auto 一并持久化到 config，
// 账号下次登录即使用新指纹。
func (c *Client) RefreshRealDeviceID(ctx context.Context, identifier string, resetProfile bool) (string, error) {
	if c == nil || c.Store == nil {
		return "", errors.New("devid: 配置存储未装配")
	}
	acc, ok := c.Store.FindAccount(identifier)
	if !ok {
		return "", fmt.Errorf("账号不存在: %s", identifier)
	}
	identifier = acc.Identifier()
	if identifier == "" {
		return "", errors.New("账号缺少可用标识")
	}

	mu := lockDevidGen(identifier)
	mu.Lock()
	defer mu.Unlock()
	if stored, ok := c.Store.EnsureAccountDeviceProfile(identifier); ok {
		acc = stored
	}

	profilePath := devid.ProfilePathFor(identifier)
	if resetProfile {
		if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("devid: 删除设备画像失败: %v", err)
		}
	}

	anchorPath, err := buildDevidAnchor(identifier, acc)
	if err != nil {
		return "", err
	}
	// The anchor is an input artifact for this one generation run; never leave
	// it behind under data/devid/anchors/ after the account is deleted.
	if anchorPath != "" {
		defer func() {
			if err := os.Remove(anchorPath); err != nil && !os.IsNotExist(err) {
				config.Logger.Warn("[real_device_id] remove anchor failed", "path", anchorPath, "error", err)
			}
		}()
	}

	genCtx, cancel := context.WithTimeout(ctx, devidGenTimeout)
	defer cancel()

	generator := c.deviceIDGenerator
	if generator == nil {
		generator = devid.Generate
	}
	deviceID, err := generator(genCtx, devid.Options{
		ProfilePath:  profilePath,
		ResetProfile: false, // 画像已在上面按需删除，这里不再重复 reset
		AnchorPath:   anchorPath,
	})
	if err != nil {
		return "", err
	}

	if err := c.Store.UpdateAccountDeviceID(identifier, deviceID, config.DeviceIDTypeAuto); err != nil {
		return "", fmt.Errorf("devid: 指纹写回配置失败: %v", err)
	}
	return deviceID, nil
}

// RefreshRealDeviceIDs 批量为账号生成真实设备指纹，实现 POST /admin/device-ids/
// real/refresh-all 的底层能力。逐账号独立成功/失败，失败不连坐。
func (c *Client) RefreshRealDeviceIDs(ctx context.Context, identifiers []string, resetProfile bool) []RealDeviceIDRefreshResult {
	results := make([]RealDeviceIDRefreshResult, len(identifiers))
	if len(identifiers) == 0 {
		return results
	}

	sem := make(chan struct{}, devidMaxConcurrency)
	var wg sync.WaitGroup
	for i, id := range identifiers {
		wg.Add(1)
		go func(idx int, identifier string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			deviceID, err := c.RefreshRealDeviceID(ctx, identifier, resetProfile)
			res := RealDeviceIDRefreshResult{Identifier: identifier, DeviceID: deviceID}
			if err != nil {
				res.DeviceID = ""
				res.Error = err.Error()
			}
			results[idx] = res
		}(i, id)
	}
	wg.Wait()
	return results
}
