package proxybridge

import (
	"context"
	"ds2api/internal/config"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func (e *instance) ensure(ctx context.Context, p config.Proxy) (port int, resultErr error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	key := config.ProxyIdentity(p)
	if err := cachedStartFailure(key); err != nil {
		return 0, err
	}
	defer func() {
		if resultErr != nil && ctx.Err() == nil {
			rememberStartFailure(key, resultErr)
		}
	}()
	if e.cmd != nil {
		select {
		case <-e.done:
			e.cmd = nil
			closeInput(e.input)
			e.input = nil
		default:
			return e.port, nil
		}
	}
	path, err := binaryPath()
	if err != nil {
		return 0, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, errors.New("无法分配本地代理端口")
	}
	port = listener.Addr().(*net.TCPAddr).Port
	if err = listener.Close(); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp("", "ds2api-proxy-")
	if err != nil {
		return 0, errors.New("无法创建代理私密配置目录")
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			config.Logger.Warn("proxy temporary directory cleanup failed", "error", err)
		}
	}()
	cfg := map[string]any{"scheme": p.Type, "server": p.Host, "server_port": p.Port, "username": p.Username, "password": p.Password, "tls_server_name": p.ServerName, "ca_file": p.CAFile, "insecure": p.Insecure, "realm": p.Realm, "mtu": p.MTU, "listen_port": port, "ready_file": filepath.Join(dir, "ready")}
	data, err := json.Marshal(cfg)
	if err != nil {
		return 0, errors.New("代理配置序列化失败")
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err = os.WriteFile(cfgPath, data, 0600); err != nil {
		return 0, errors.New("无法写入代理私密配置")
	}
	if len(data) > 65536 {
		return 0, errors.New("代理转换配置超过 64 KiB")
	}
	cmd := exec.Command(path, "-config", cfgPath, "-watch-parent")
	input, err := cmd.StdinPipe()
	if err != nil {
		return 0, errors.New("无法创建代理生命周期管道")
	}
	diagnostics := &helperDiagnostics{}
	e.diagnostics = diagnostics
	cmd.Stdout, cmd.Stderr = diagnostics, io.Discard
	if err = cmd.Start(); err != nil {
		closeInput(input)
		return 0, errors.New("无法启动 TURN/SSTP 转换器")
	}
	e.cmd, e.port, e.done, e.input = cmd, port, make(chan struct{}), input
	done := e.done
	go func() {
		if err := cmd.Wait(); err != nil {
			config.Logger.Debug("proxy helper exited", "proxy_id", p.ID)
		}
		close(done)
	}()
	timeout := 15 * time.Second
	if p.Type == "sstp" {
		timeout = 35 * time.Second
	}
	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			closeInput(e.input)
			e.input = nil
			e.cmd = nil
			return 0, errors.New(diagnostics.failure("TURN/SSTP 启动失败，请检查服务器、证书、凭据及 RFC 6062 / MS-CHAPv2 支持"))
		case <-startCtx.Done():
			e.stop()
			return 0, fmt.Errorf("TURN/SSTP 启动超时或请求已取消: %w", startCtx.Err())
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
				select {
				case <-done:
					closeInput(e.input)
					e.input = nil
					e.cmd = nil
					return 0, errors.New("代理转换器已退出")
				default:
					return port, nil
				}
			}
		}
	}
}
func (e *instance) stop() {
	closeInput(e.input)
	e.input = nil
	if e.cmd == nil {
		return
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-e.done:
	case <-timer.C:
		if err := e.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			config.Logger.Warn("proxy helper cleanup failed", "error", err)
		}
		<-e.done
	}
	e.cmd = nil
}
func closeInput(input io.WriteCloser) {
	if input != nil {
		if err := input.Close(); err != nil {
			config.Logger.Debug("proxy lifecycle pipe cleanup failed", "error", err)
		}
	}
}
