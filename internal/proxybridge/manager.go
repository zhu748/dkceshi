// Package proxybridge manages the isolated TURN/SSTP protocol helper.
package proxybridge

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"ds2api/internal/config"
	"golang.org/x/net/proxy"
)

type instance struct {
	mu       sync.Mutex
	refs     atomic.Int64
	last     atomic.Int64
	cmd      *exec.Cmd
	done     chan struct{}
	port     int
	lifetime context.Context
	cancel   context.CancelFunc
	input    io.WriteCloser
}

var registry = struct {
	sync.Mutex
	items map[string]*instance
	once  sync.Once
}{items: map[string]*instance{}}

func binaryPath() (string, error) {
	if p := os.Getenv("DS2API_PROXY_BRIDGE_BIN"); p != "" {
		return p, nil
	}
	name := "ds2api-proxy-bridge"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, _ := os.Executable()
	for _, p := range []string{filepath.Join(filepath.Dir(exe), name), filepath.Join("bin", name)} {
		if info, e := os.Stat(p); e == nil && !info.IsDir() {
			return filepath.Abs(p)
		}
	}
	if p, e := exec.LookPath(name); e == nil {
		return p, nil
	}
	return "", errors.New("TURN/SSTP 转换器未安装，请使用包含 ds2api-proxy-bridge 的部署包或设置 DS2API_PROXY_BRIDGE_BIN")
}
func Dial(ctx context.Context, p config.Proxy, network, target string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("TURN/SSTP 仅支持 TCP")
	}
	registry.once.Do(func() { go reap() })
	key := config.ProxyIdentity(p)
	registry.Lock()
	e := registry.items[key]
	var retired *instance
	if e == nil {
		if len(registry.items) >= 128 {
			retired = evictIdleLocked()
			if retired == nil {
				registry.Unlock()
				return nil, errors.New("代理转换实例达到上限，当前实例均在使用中")
			}
		}
		lifetime, cancel := context.WithCancel(context.Background())
		e = &instance{lifetime: lifetime, cancel: cancel}
		registry.items[key] = e
	}
	e.refs.Add(1)
	registry.Unlock()
	if retired != nil {
		retired.stop()
		retired.mu.Unlock()
	}
	release := func() {
		e.last.Store(time.Now().UnixNano())
		if e.refs.Add(-1) == 0 {
			discardStopped(key, e)
		}
	}
	dialCtx, cancel := context.WithCancel(ctx)
	stopLifetime := context.AfterFunc(e.lifetime, cancel)
	defer cancel()
	defer stopLifetime()
	ctx = dialCtx
	if err := e.lockContext(ctx); err != nil {
		release()
		return nil, err
	}
	port, err := e.ensure(ctx, p)
	e.mu.Unlock()
	if err != nil {
		release()
		return nil, err
	}
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil, &net.Dialer{Timeout: 15 * time.Second})
	if err != nil {
		release()
		return nil, errors.New("本地代理转换连接初始化失败")
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, 35*time.Second)
	defer cancelConnect()
	conn, err := d.(proxy.ContextDialer).DialContext(connectCtx, "tcp", target)
	if err != nil {
		release()
		if connectCtx.Err() != nil {
			return nil, connectCtx.Err()
		}
		return nil, errors.New("TURN/SSTP 代理目标连接失败，请检查 TCP 中继支持、出口策略及凭据")
	}
	return &leasedConn{Conn: conn, release: release}, nil
}

type leasedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *leasedConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
