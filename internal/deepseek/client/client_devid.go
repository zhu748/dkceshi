package client

import (
	"context"
	"ds2api/internal/config"
	"ds2api/internal/devid"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const devidGenTimeout = 150 * time.Second

func (c *Client) DeviceIDReadiness(ctx context.Context) devid.Readiness {
	return c.deviceReadiness.Check(ctx)
}

// RefreshRealDeviceID stages archives and commits only a result issued for the
// account's current device and routing identity.
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
	genCtx, cancel := context.WithTimeout(ctx, devidGenTimeout)
	defer cancel()
	unlock, err := devid.AcquireAccount(genCtx, identifier)
	if err != nil {
		return "", err
	}
	defer unlock()
	_, ok = c.Store.EnsureAccountDeviceProfile(identifier)
	if !ok {
		return "", fmt.Errorf("账号不存在: %s", identifier)
	}
	snapshot, ok := c.Store.DeviceSnapshot(identifier)
	if !ok {
		return "", fmt.Errorf("账号不存在: %s", identifier)
	}
	acc = snapshot.Account
	release, err := devid.AcquireGeneration(genCtx)
	if err != nil {
		return "", err
	}
	defer release()
	update, err := devid.PrepareProfile(identifier, c.Store.Accounts(), resetProfile)
	if err != nil {
		return "", err
	}
	defer update.Close()
	anchorPath, err := buildDevidAnchor(identifier, acc)
	if err != nil {
		return "", err
	}
	if anchorPath != "" {
		defer removeDevidAnchor(anchorPath)
	}
	generator := c.deviceIDGenerator
	if generator == nil {
		generator = devid.Generate
	}
	reporter, closeReporter, err := c.deviceReporter(acc)
	if err != nil {
		return "", err
	}
	defer closeReporter()
	deviceID, err := generator(genCtx, devid.Options{ProfilePath: update.Path, AnchorPath: anchorPath, Reporter: reporter})
	if err != nil {
		return "", err
	}
	if err := c.Store.CommitGeneratedDeviceID(genCtx, snapshot, deviceID, update.Promote); err != nil {
		return "", fmt.Errorf("devid: 指纹写回配置失败: %w", err)
	}
	update.Accept()
	return deviceID, nil
}

func removeDevidAnchor(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		config.Logger.Warn("[real_device_id] remove anchor failed", "path", path, "error", err)
	}
}

// A bounded worker pool avoids creating one blocked goroutine per account.
// The global generator limit also covers single refreshes and login calls.
func (c *Client) RefreshRealDeviceIDs(ctx context.Context, identifiers []string, resetProfile bool) []RealDeviceIDRefreshResult {
	results := make([]RealDeviceIDRefreshResult, len(identifiers))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(len(identifiers), devid.MaxConcurrency) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				id, err := c.RefreshRealDeviceID(ctx, identifiers[i], resetProfile)
				results[i] = RealDeviceIDRefreshResult{Identifier: identifiers[i], DeviceID: id}
				if err != nil {
					results[i].Error = err.Error()
					results[i].DeviceID = ""
				}
			}
		}()
	}
	for i := range identifiers {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}
