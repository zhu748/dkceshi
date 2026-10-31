package client

import (
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// androidVersionByAPI 把 android_api_level 映射成 Android 主版本号（UA 里的
// "Android 16" 与 Chrome-on-Android 指纹的 OS 版本必须同源）。不支持的 level 明确报错，避免锚定到另一代系统。
var androidVersionByAPI = map[string]int{
	"24": 7, "25": 7, "26": 8, "27": 8, "28": 9, "29": 10, "30": 11,
	"31": 12, "32": 12, "33": 13, "34": 14, "35": 15, "36": 16, "37": 17,
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
		return "", fmt.Errorf("devid: unsupported Android API level %q (supported: 24-37)", apiLevel)
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
	dir := filepath.Join(config.DevidStateDir(), "anchors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("devid: 创建锚定目录失败: %v", err)
	}
	file, err := os.CreateTemp(dir, config.AccountStorageKey(identifier)+"-*.json")
	if err != nil {
		return "", err
	}
	p := file.Name()
	_, writeErr := file.Write(raw)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		removeDevidAnchor(p)
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
