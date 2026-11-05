package proxies

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

var proxyConnectivityTester = func(ctx context.Context, proxy config.Proxy) map[string]any {
	return dsclient.TestProxyConnectivity(ctx, proxy)
}

func validateProxyMutation(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if err := config.ValidateProxyConfig(cfg.Proxies); err != nil {
		return err
	}
	return config.ValidateAccountProxyReferences(cfg.Accounts, cfg.Proxies)
}

func proxyResponse(proxy config.Proxy) map[string]any { return config.PublicProxy(proxy) }
func (h *Handler) listProxies(w http.ResponseWriter, _ *http.Request) {
	proxies := proxySnapshot(h.Store)
	items := make([]map[string]any, 0, len(proxies))
	for _, p := range proxies {
		items = append(items, config.PublicProxy(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) addProxy(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if !decodeProxyRequest(w, r, &req) {
		return
	}
	proxy := toProxy(req)
	if config.IsMihomoManagedProxyID(proxy.ID) {
		writeJSON(w, 400, map[string]any{"detail": "mihomo- 前缀由代理桥保留"})
		return
	}
	if fieldString(req, "id") == "" {
		proxy.ID = "proxy_" + config.ProxyIdentity(proxy)[:24]
	}
	skipped := false
	err := h.Store.Update(func(c *config.Config) error {
		if err := config.ValidateProxyConfig([]config.Proxy{proxy}); err != nil {
			return err
		}
		identity := config.ProxyIdentity(proxy)
		for _, existing := range c.Proxies {
			if config.ProxyIdentity(existing) == identity {
				proxy = existing
				skipped = true
				return nil
			}
		}
		c.Proxies = append(c.Proxies, proxy)
		return validateProxyMutation(c)
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	if !skipped {
		h.ProxyChanged()
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "skipped": skipped, "proxy": proxyResponse(proxy)})
}

// ProxyChanged 在代理新增/变更后刷新运行侧缓存（账号池 + DeepSeek client 代理连接池）。
func (h *Handler) ProxyChanged() {
	if h == nil {
		return
	}
	if h.Pool != nil {
		h.Pool.Reset()
	}
	if h.ResetProxyClients != nil {
		h.ResetProxyClients()
	}
}

func (h *Handler) updateProxy(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "proxyID")
	if decoded, err := url.PathUnescape(proxyID); err == nil {
		proxyID = decoded
	}
	if config.IsMihomoManagedProxyID(strings.TrimSpace(proxyID)) {
		writeJSON(w, 409, map[string]any{"detail": "该节点由代理桥管理，请在代理桥页面修改或删除"})
		return
	}
	var req map[string]any
	if !decodeProxyRequest(w, r, &req) {
		return
	}
	proxy := toProxy(req)
	proxy.ID = strings.TrimSpace(proxyID)

	err := h.Store.Update(func(c *config.Config) error {
		for i, existing := range c.Proxies {
			existing = config.NormalizeProxy(existing)
			if existing.ID != proxy.ID {
				continue
			}
			if proxy.Password == "" && req["clear_password"] != true {
				proxy.Password = existing.Password
			}
			c.Proxies[i] = proxy
			return validateProxyMutation(c)
		}
		return newRequestError("代理不存在")
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.ProxyChanged()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "proxy": proxyResponse(proxy)})
}

func (h *Handler) deleteProxy(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "proxyID")
	if decoded, err := url.PathUnescape(proxyID); err == nil {
		proxyID = decoded
	}
	if config.IsMihomoManagedProxyID(strings.TrimSpace(proxyID)) {
		writeJSON(w, 409, map[string]any{"detail": "该节点由代理桥管理，请在代理桥页面修改或删除"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, existing := range c.Proxies {
			existing = config.NormalizeProxy(existing)
			if existing.ID == strings.TrimSpace(proxyID) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return newRequestError("代理不存在")
		}
		c.Proxies = append(c.Proxies[:idx], c.Proxies[idx+1:]...)
		for i := range c.Accounts {
			if strings.TrimSpace(c.Accounts[i].ProxyID) == strings.TrimSpace(proxyID) {
				c.Accounts[i].ProxyID = ""
				// 代理已删除，手动绑定不再有对象：清除标记让账号回到可调度状态。
				c.Accounts[i].ManualBinding = false
			}
		}
		return validateProxyMutation(c)
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.ProxyChanged()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *Handler) testProxy(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if !decodeProxyRequest(w, r, &req, "proxy_id", "base_proxy_id") {
		return
	}
	proxyID := fieldString(req, "proxy_id")

	var proxy config.Proxy
	if proxyID != "" {
		if len(req) != 1 {
			writeJSON(w, 400, map[string]any{"detail": "测试已保存代理时不能同时覆盖代理字段"})
			return
		}
		var ok bool
		proxy, ok = findProxyByID(h.Store.Snapshot(), proxyID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"detail": "代理不存在"})
			return
		}
	} else {
		proxy = toProxy(req)
		if baseID := fieldString(req, "base_proxy_id"); baseID != "" {
			existing, ok := findProxyByID(h.Store.Snapshot(), baseID)
			if !ok {
				writeJSON(w, 404, map[string]any{"detail": "代理不存在"})
				return
			}
			if proxy.Password == "" && req["clear_password"] != true {
				proxy.Password = existing.Password
			}
		}
	}

	result := proxyConnectivityTester(r.Context(), proxy)
	writeJSON(w, http.StatusOK, result)
}
