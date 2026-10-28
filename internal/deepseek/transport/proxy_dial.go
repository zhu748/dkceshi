package transport

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

// Environment proxies are used only when no account-specific dialer was given.
// Establish the tunnel first so TLS and the ordered HTTP/2 stream use the same
// route as the account's normal requests.
func environmentProxyDialer(dial DialContextFunc) DialContextFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		proxyURL, err := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: "https", Host: addr}})
		if err != nil {
			return nil, err
		}
		if proxyURL == nil {
			return dial(ctx, network, addr)
		}
		if proxyURL.Scheme == "socks5" || proxyURL.Scheme == "socks5h" {
			d, err := proxy.FromURL(proxyURL, contextDialer{dial})
			if err != nil {
				return nil, err
			}
			if d, ok := d.(proxy.ContextDialer); ok {
				return d.DialContext(ctx, network, addr)
			}
			return nil, errors.New("proxy dialer does not support request cancellation")
		}
		if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
			return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
		}
		return dialConnectTunnel(ctx, dial, proxyURL, addr)
	}
}

type contextDialer struct{ dial DialContextFunc }

func (d contextDialer) Dial(network, addr string) (net.Conn, error) {
	return d.dial(context.Background(), network, addr)
}
func (d contextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dial(ctx, network, addr)
}

func dialConnectTunnel(ctx context.Context, dial DialContextFunc, proxyURL *url.URL, target string) (net.Conn, error) {
	addr := proxyURL.Host
	if proxyURL.Port() == "" {
		port := "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
		addr = net.JoinHostPort(proxyURL.Hostname(), port)
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { closeTransportConn(rawConn) })
	defer stop()
	if proxyURL.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: proxyURL.Hostname()})
		if err := secure.HandshakeContext(ctx); err != nil {
			return nil, errors.Join(err, conn.Close())
		}
		conn = secure
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)))
	}
	if err := request.Write(conn); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.Join(fmt.Errorf("proxy CONNECT returned HTTP %d", response.StatusCode), conn.Close())
	}
	if reader.Buffered() > 0 {
		return &bufferedProxyConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedProxyConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
