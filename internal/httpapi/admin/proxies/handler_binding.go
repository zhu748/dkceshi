package proxies

import (
	"ds2api/internal/config"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
	"strings"
)

// updateAccountProxy 更新账号的代理/节点绑定。三种语义：
//   - body {"proxy_id":"<id>"}：绑定手动代理（非空）或解绑直连（空串）；
//   - body {"mode":"auto"}：把账号交回代理桥自动调度——清空绑定并同时清除
//     no_proxy 与 manual_binding，下一次绑定调和会给它分配健康节点。
//     这是手动绑定之后回到自动分配的唯一入口。
//
// 非空的 proxy_id 表示用户显式指定代理由用户掌握：账号退出自动调度
// （故障转移/补位都不再改动它）；手动选定的 mihomo 节点同样落在这条语义上
// （前端走 /admin/mihomo/bindings 时由 BindAccount 置位，两条路径一致）。
func (h *Handler) updateAccountProxy(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")
	if decoded, err := url.PathUnescape(identifier); err == nil {
		identifier = decoded
	}
	var req map[string]any
	if !decodeRequest(w, r, &req, "proxy_id", "mode") {
		return
	}
	proxyID := strings.TrimSpace(fieldString(req, "proxy_id"))
	autoMode := strings.EqualFold(strings.TrimSpace(fieldString(req, "mode")), "auto")

	_, hasID := req["proxy_id"]
	_, hasMode := req["mode"]
	if (hasMode && !autoMode) || (!autoMode && !hasID) || (autoMode && proxyID != "") {
		writeJSON(w, 400, map[string]any{"detail": "请明确指定 proxy_id（空字符串表示直连），或单独使用 mode=auto"})
		return
	}

	err := h.Store.Update(func(c *config.Config) error {
		if proxyID != "" {
			if _, ok := findProxyByID(*c, proxyID); !ok {
				return newRequestError("代理不存在")
			}
		}
		for i, acc := range c.Accounts {
			if !accountMatchesIdentifier(acc, identifier) {
				continue
			}
			if autoMode {
				c.Accounts[i].ProxyID = ""
				c.Accounts[i].NoProxy = false       // 不再强制直连
				c.Accounts[i].ManualBinding = false // 撤销手动绑定，重新纳入自动调度
				return validateProxyMutation(c)
			}
			c.Accounts[i].ProxyID = proxyID
			// 显式选择"不代理"（proxy_id 为空）后，自动调度不再给该账号分配节点。
			c.Accounts[i].NoProxy = proxyID == ""
			c.Accounts[i].ManualBinding = proxyID != ""
			return validateProxyMutation(c)
		}
		return newRequestError("账号不存在")
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.ProxyChanged()
	if autoMode {
		// 账号刚回到自动调度：立即按已有测速结果补位分配，不必等下一轮巡检。
		// 桥未装配时回调为 nil（见 deps.go），与账号侧 notifyAccountsChanged 同规则跳过。
		if h.OnAccountsChanged != nil {
			h.OnAccountsChanged()
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "proxy_id": "", "mode": "auto"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "proxy_id": proxyID})
}
