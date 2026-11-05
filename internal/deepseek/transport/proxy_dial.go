package transport

import (
	"context"
	"ds2api/internal/config"
	"ds2api/internal/proxydial"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

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
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
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
	p := config.Proxy{Type: proxyURL.Scheme, Host: proxyURL.Hostname(), Port: config.ProxyDefaultPort(proxyURL.Scheme)}
	if proxyURL.Port() != "" {
		var err error
		p.Port, err = strconv.Atoi(proxyURL.Port())
		if err != nil {
			return nil, err
		}
	}
	if proxyURL.User != nil {
		p.Username = proxyURL.User.Username()
		p.Password, _ = proxyURL.User.Password()
	}
	return proxydial.ConnectHTTP(ctx, proxydial.DialFunc(dial), p, "tcp", target)
}
