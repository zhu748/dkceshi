package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/devid"
)

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

	profilePath := devid.ProfilePathFor(identifier)
	if resetProfile {
		if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("devid: 删除设备画像失败: %v", err)
		}
	}

	genCtx, cancel := context.WithTimeout(ctx, devidGenTimeout)
	defer cancel()

	deviceID, err := devid.Generate(genCtx, devid.Options{
		ProfilePath:  profilePath,
		ResetProfile: false, // 画像已在上面按需删除，这里不再重复 reset
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
