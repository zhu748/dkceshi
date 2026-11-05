package proxydial

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"ds2api/internal/config"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func tlsConfig(p config.Proxy) (*tls.Config, error) {
	if p.Type != "https" {
		return nil, nil
	}
	name := p.ServerName
	if name == "" {
		name = p.Host
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name, InsecureSkipVerify: p.Insecure}
	if p.CAFile != "" {
		data, err := os.ReadFile(p.CAFile)
		if err != nil {
			return nil, fmt.Errorf("无法读取代理 CA 证书")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("代理 CA 证书格式无效")
		}
		cfg.RootCAs = roots
	}
	return cfg, nil
}

// ConnectHTTP shares bounded CONNECT handling with environment proxies.
func ConnectHTTP(ctx context.Context, dial DialFunc, p config.Proxy, network, target string) (net.Conn, error) {
	p = config.NormalizeProxy(p)
	if err := config.ValidateProxyConfig([]config.Proxy{p}); err != nil {
		return nil, err
	}
	cfg, err := tlsConfig(p)
	if err != nil {
		return nil, err
	}
	return connect(ctx, dial, p, cfg, network, target)
}

func connect(ctx context.Context, dial DialFunc, p config.Proxy, cfg *tls.Config, network, target string) (result net.Conn, resultErr error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("HTTP 代理仅支持 TCP")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	defer func() {
		resultErr = contextFailure(ctx, resultErr)
	}()
	conn, err := dial(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	if err != nil {
		return nil, err
	}
	raw := conn
	stop := context.AfterFunc(ctx, func() { closeConn(raw) })
	defer stop()
	success := false
	defer func() {
		if !success {
			closeConn(conn)
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		if err = conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	if cfg != nil {
		secured := tls.Client(conn, cfg)
		conn = secured
		if err = secured.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("HTTPS 代理 TLS 握手失败: %w", err)
		}
	}
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if p.Username != "" {
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(p.Username+":"+p.Password)))
	}
	if err = req.Write(conn); err != nil {
		return nil, err
	}
	reader, err := readConnectResponse(conn, req)
	if err != nil {
		return nil, err
	}
	if !stop() || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	success = true
	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
