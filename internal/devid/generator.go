// Package devid 通过真实浏览器运行数美 fp Web SDK，为账号签发上游认可的
// 真实设备指纹（device_id）。
//
// 背景：DeepSeek 登录接口会校验 device_id 的真实性，本地随机/派生指纹一律被
// RISK_DEVICE_DETECTED 拒绝。真实指纹只能由数美云端为 DeepSeek 组织
// （见登录页 _smConf）签发。本项目以无头 Chromium 加载 chat.deepseek.com
// 登录页，让页面内嵌的数美 SDK 在真实浏览器环境里完成采集与上报，再读取
// SMSdk.getDeviceId() 的结果。
//
// 生成器本体是 scripts/devid-generate.cjs（Node + Playwright），本包只负责
// 解析脚本位置、拼接参数并执行解析输出。浏览器画像（storageState）持久化在
// DevidDir()/profiles 下，按账号一一对应：复用画像 = 保持同一设备身份换新指纹；
// 删除画像 = 换一台全新设备。
package devid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ds2api/internal/config"
)

// defaultTimeout 是单次指纹生成的总超时。正常路径 10~25 秒（WAF 挑战 + SDK
// 采集上报），给重试与慢网络留出余量。
const defaultTimeout = 120 * time.Second

// Options 描述一次指纹生成的全部输入。
type Options struct {
	// Script 是生成脚本路径。空串时按 LocateScript() 解析。
	Script string
	// Node 是 node 可执行文件。空串时使用 PATH 上的 node。
	Node string
	// ProfilePath 是浏览器画像文件（storageState）。存在且未 Reset 时会被
	// 复用（同一设备身份）；生成成功后写回该文件。
	ProfilePath string
	// ResetProfile 为 true 时忽略并删除既有画像，等于换一台新设备。
	ResetProfile bool
	// Timeout 是总超时；零值取 defaultTimeout。
	Timeout time.Duration
	// ExtraEnv 是附加环境变量（如 PLAYWRIGHT_MODULE）。
	ExtraEnv []string
}

// LocateScript 按优先级解析生成脚本路径：
//  1. 环境变量 DS2API_DEVID_SCRIPT
//  2. DevidDir()/generate.cjs（数美扩展包形态，与"仓库只保留加载代码"的
//     原设计一致，便于热替换脚本）
//  3. BaseDir()/scripts/devid-generate.cjs（仓库内置副本）
func LocateScript() string {
	if v := strings.TrimSpace(os.Getenv("DS2API_DEVID_SCRIPT")); v != "" {
		return v
	}
	packaged := filepath.Join(config.DevidDir(), "generate.cjs")
	if fileReadable(packaged) {
		return packaged
	}
	return filepath.Join(config.BaseDir(), "scripts", "devid-generate.cjs")
}

// Generate 运行指纹生成脚本，返回数美签发的真实 device_id。
func Generate(ctx context.Context, opts Options) (string, error) {
	script := strings.TrimSpace(opts.Script)
	if script == "" {
		script = LocateScript()
	}
	if !fileReadable(script) {
		return "", fmt.Errorf("devid: 生成脚本不存在: %s", script)
	}

	node := strings.TrimSpace(opts.Node)
	if node == "" {
		node = strings.TrimSpace(os.Getenv("DS2API_DEVID_NODE"))
	}
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			return "", errors.New("devid: 找不到 node 可执行文件（指纹生成需要 Node + Playwright）")
		}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{script, "--timeout", fmt.Sprintf("%d", int(timeout.Seconds()))}
	if opts.ResetProfile {
		args = append(args, "--reset")
	}
	if strings.TrimSpace(opts.ProfilePath) != "" {
		args = append(args, "--profile", opts.ProfilePath)
	}

	cmd := exec.CommandContext(runCtx, node, args...)
	cmd.Env = append(os.Environ(), opts.ExtraEnv...)
	// stdout 只承载结果 JSON；stderr 的过程日志转发到服务日志。
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if runCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("devid: 指纹生成超时（%s）", timeout)
		}
		if detail != "" {
			lines := strings.Split(detail, "\n")
			if len(lines) > 3 {
				lines = lines[len(lines)-3:]
			}
			detail = strings.Join(lines, " | ")
		}
		return "", fmt.Errorf("devid: 生成脚本执行失败: %v %s", err, detail)
	}

	var result struct {
		OK       bool   `json:"ok"`
		DeviceID string `json:"device_id"`
		Error    string `json:"error"`
	}
	line := strings.TrimSpace(lastJSONLine(stdout.String()))
	if line == "" {
		return "", fmt.Errorf("devid: 生成脚本无输出")
	}
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		return "", fmt.Errorf("devid: 生成脚本输出解析失败: %v", err)
	}
	if !result.OK || strings.TrimSpace(result.DeviceID) == "" {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = "未知错误"
		}
		return "", fmt.Errorf("devid: %s", msg)
	}
	return strings.TrimSpace(result.DeviceID), nil
}

// ProfilePathFor 返回账号对应的浏览器画像文件路径。
func ProfilePathFor(identifier string) string {
	return filepath.Join(config.DevidDir(), "profiles", config.SanitizeFilename(identifier)+".json")
}

// lastJSONLine 从脚本输出中取最后一行非空文本。脚本保证 stdout 只有结果 JSON，
// 这里只是防御性地容忍尾部多余空白与换行。
func lastJSONLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

func fileReadable(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
