// Package proxydial provides account-specific TCP tunnels without direct fallback.
package proxydial

import (
	"context"
	"ds2api/internal/config"
	"ds2api/internal/proxybridge"
	"fmt"
	"golang.org/x/net/proxy"
	"net"
	"strconv"
	"time"
)

type DialFunc func(context.Context, string, string) (net.Conn, error)

func New(p config.Proxy) (DialFunc, error) {
	p = config.NormalizeProxy(p)
	if err := config.ValidateProxyConfig([]config.Proxy{p}); err != nil {
		return nil, err
	}
	forward := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	switch p.Type {
	case "turn", "turns", "sstp":
		return func(ctx context.Context, network, target string) (net.Conn, error) {
			return proxybridge.Dial(ctx, p, network, target)
		}, nil
	case "http", "https":
		cfg, err := tlsConfig(p)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, network, target string) (net.Conn, error) {
			return connect(ctx, forward, p, cfg, network, target)
		}, nil
	case "socks4", "socks4a":
		return func(ctx context.Context, network, target string) (net.Conn, error) {
			return socks4(ctx, forward, p, network, target)
		}, nil
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if p.Username != "" || p.Password != "" {
			auth = &proxy.Auth{User: p.Username, Password: p.Password}
		}
		d, err := proxy.SOCKS5("tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), auth, forward)
		if err != nil {
			return nil, fmt.Errorf("SOCKS5 拨号器初始化失败")
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("代理不支持请求取消")
		}
		return func(ctx context.Context, network, target string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			conn, err := dialSOCKS5(ctx, p.Type, network, target, nil, cd.DialContext)
			return conn, contextFailure(ctx, err)
		}, nil
	}
	return nil, fmt.Errorf("不支持的代理协议")
}
func closeConn(c net.Conn) {
	if err := c.Close(); err != nil {
		config.Logger.Debug("proxy connection cleanup failed", "error", err)
	}
}

func contextFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Socket deadlines can fire just before the context timer is scheduled.
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return err
}
