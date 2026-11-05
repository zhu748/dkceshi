package client

import (
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/proxydial"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

type requestClients struct {
	regular   trans.Doer
	stream    trans.Doer
	fallback  trans.Doer
	fallbackS trans.Doer
}

var proxyConnectivityTestURL = "https://chat.deepseek.com/"

func proxyCacheKey(p config.Proxy) string {
	p = config.NormalizeProxy(p)
	return p.ID + "|" + config.ProxyIdentity(p)
}

func proxyDialContext(proxyCfg config.Proxy) (trans.DialContextFunc, error) {
	dial, err := proxydial.New(proxyCfg)
	return trans.DialContextFunc(dial), err
}

func (c *Client) defaultRequestClients() requestClients {
	if c.clientsClosed() {
		return failedProxyClients(errClientClosed)
	}
	return requestClients{
		regular:   c.regular,
		stream:    c.stream,
		fallback:  c.fallback,
		fallbackS: c.fallbackS,
	}
}

func (c *Client) resolveProxyForAccount(acc config.Account) (config.Proxy, bool) {
	if c == nil || c.Store == nil || acc.NoProxy {
		return config.Proxy{}, false
	}
	proxyID := strings.TrimSpace(acc.ProxyID)
	if proxyID == "" {
		return config.Proxy{}, false
	}
	// Direct lookup instead of a full-config Snapshot deep copy; this runs on
	// every upstream request (session, PoW, completion, upload...).
	return c.Store.ProxyByID(proxyID)
}

func (c *Client) requestClientsFromContext(ctx context.Context) requestClients {
	if a, ok := auth.FromContext(ctx); ok {
		return c.requestClientsForAccount(a.Account)
	}
	return c.defaultRequestClients()
}

func (c *Client) requestClientsForAuth(ctx context.Context, a *auth.RequestAuth) requestClients {
	if a != nil {
		return c.requestClientsForAccount(a.Account)
	}
	return c.requestClientsFromContext(ctx)
}

func (c *Client) requestClientsForAccount(acc config.Account) requestClients {
	return c.requestClientsWithDialer(acc, proxyDialContext)
}

// Epoch checks prevent a slow dialer build from repopulating a reset cache.
func (c *Client) requestClientsWithDialer(acc config.Account, build func(config.Proxy) (trans.DialContextFunc, error)) requestClients {
	for attempt := 0; attempt < 3; attempt++ {
		c.proxyClientsMu.RLock()
		epoch, closed := c.proxyEpoch, c.proxyClosed
		c.proxyClientsMu.RUnlock()
		if closed {
			return failedProxyClients(errClientClosed)
		}
		proxyCfg, proxied := c.resolveProxyForAccount(acc)
		if !proxied && acc.ProxyID != "" && !acc.NoProxy {
			return failedProxyClients(fmt.Errorf("账号绑定的代理不可用"))
		}
		if !proxied && !acc.NoProxy {
			return c.defaultRequestClients()
		}
		key := "direct"
		if proxied {
			key = proxyCacheKey(proxyCfg)
		}
		c.proxyClientsMu.RLock()
		cached, ok := c.proxyClients[key]
		current := epoch == c.proxyEpoch && !c.proxyClosed
		c.proxyClientsMu.RUnlock()
		if !current {
			continue
		}
		if ok {
			return c.reportingBundle(proxyCfg.ID, cached)
		}
		dial := trans.DialContextFunc((&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext)
		if proxied {
			var err error
			dial, err = build(proxyCfg)
			if err != nil {
				c.proxyClientsMu.RLock()
				current = epoch == c.proxyEpoch && !c.proxyClosed
				c.proxyClientsMu.RUnlock()
				if !current {
					continue
				}
				return failedProxyClients(err)
			}
		}
		c.proxyClientsMu.Lock()
		if epoch != c.proxyEpoch || c.proxyClosed {
			c.proxyClientsMu.Unlock()
			continue
		}
		if cached, ok = c.proxyClients[key]; !ok {
			cached = requestClients{regular: trans.NewWithDialContext(60*time.Second, dial), stream: trans.NewWithDialContext(0, dial), fallback: trans.NewFallbackClient(60*time.Second, dial), fallbackS: trans.NewFallbackClient(0, dial)}
			if c.proxyClients == nil {
				c.proxyClients = map[string]requestClients{}
			}
			c.proxyClients[key] = cached
		}
		c.proxyClientsMu.Unlock()
		return c.reportingBundle(proxyCfg.ID, cached)
	}
	return failedProxyClients(fmt.Errorf("代理配置连续变更，请重试"))
}

func applyProxyConnectivityHeaders(req *http.Request) {
	if req == nil {
		return
	}
	for key, value := range dsprotocol.BaseHeaders {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	if req.Header.Get("x-rangers-id") == "" {
		req.Header.Set("x-rangers-id", dsprotocol.BaseHeaders["x-rangers-id"])
	}
}

func proxyConnectivityStatus(statusCode int) (bool, string) {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return true, fmt.Sprintf("代理可达，目标返回 HTTP %d", statusCode)
	case statusCode >= 300 && statusCode < 500:
		return true, fmt.Sprintf("代理可达，但目标返回 HTTP %d（可能是风控或挑战）", statusCode)
	default:
		return false, fmt.Sprintf("目标返回 HTTP %d", statusCode)
	}
}

func TestProxyConnectivity(ctx context.Context, proxyCfg config.Proxy) map[string]any {
	start := time.Now()
	proxyCfg = config.NormalizeProxy(proxyCfg)
	result := map[string]any{
		"success":       false,
		"proxy_id":      proxyCfg.ID,
		"proxy_type":    proxyCfg.Type,
		"response_time": 0,
	}

	if err := config.ValidateProxyConfig([]config.Proxy{proxyCfg}); err != nil {
		result["message"] = "代理配置无效: " + err.Error()
		return result
	}
	dialContext, err := proxyDialContext(proxyCfg)
	if err != nil {
		result["message"] = "代理拨号器初始化失败: " + err.Error()
		return result
	}

	timeout := 15 * time.Second
	if proxyCfg.Type == "sstp" {
		timeout = 60 * time.Second
	}
	client := trans.NewFallbackClient(timeout, dialContext)
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyConnectivityTestURL, nil)
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	applyProxyConnectivityHeaders(req)

	resp, err := client.Do(req)
	result["response_time"] = int(time.Since(start).Milliseconds())
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[proxy] close response body failed", "proxy_id", proxyCfg.ID, "error", closeErr)
		}
	}()

	result["status_code"] = resp.StatusCode
	result["success"], result["message"] = proxyConnectivityStatus(resp.StatusCode)
	return result
}

type failedProxyDoer struct{ err error }

func (d failedProxyDoer) Do(*http.Request) (*http.Response, error) { return nil, d.err }
func failedProxyClients(err error) requestClients {
	d := failedProxyDoer{err}
	return requestClients{regular: d, stream: d, fallback: d, fallbackS: d}
}
