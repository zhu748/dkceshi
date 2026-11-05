package proxies

import (
	"bytes"
	"ds2api/internal/config"
	"ds2api/internal/proxyinput"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func decodeRequest(w http.ResponseWriter, r *http.Request, v any, fields ...string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8*proxyinput.MaxBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"detail": "请求超出大小限制"})
		return false
	}
	decoded, err := proxyinput.DecodeJSON(string(data))
	if err != nil {
		writeJSON(w, 400, map[string]any{"detail": err.Error()})
		return false
	}
	if _, ok := decoded.(map[string]any); !ok {
		writeJSON(w, 400, map[string]any{"detail": "请求 JSON 必须为对象"})
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeJSON(w, 400, map[string]any{"detail": "请求 JSON 字段未知或类型无效"})
		return false
	}
	// Validate field types before the legacy map-to-config conversion.
	if m, ok := v.(*map[string]any); ok {
		allowed := map[string]bool{"id": true, "name": true, "type": true, "host": true, "port": true, "username": true, "password": true, "tls_server_name": true, "ca_file": true, "insecure": true, "realm": true, "mtu": true, "clear_password": true, "proxy_id": true, "mode": true}
		if len(fields) > 0 {
			allowed = map[string]bool{}
			for _, field := range fields {
				allowed[field] = true
			}
		}
		for key, value := range *m {
			if !allowed[key] {
				writeJSON(w, 400, map[string]any{"detail": "请求包含未知字段"})
				return false
			}
			switch key {
			case "port", "mtu":
				n, ok := value.(float64)
				if !ok || n != float64(int(n)) {
					writeJSON(w, 400, map[string]any{"detail": "端口和 MTU 必须为整数"})
					return false
				}
			case "insecure", "clear_password":
				if _, ok := value.(bool); !ok {
					writeJSON(w, 400, map[string]any{"detail": "开关字段必须为布尔值"})
					return false
				}
			default:
				if _, ok := value.(string); !ok {
					writeJSON(w, 400, map[string]any{"detail": "文本字段必须为字符串"})
					return false
				}
			}
		}
	}
	return true
}
func (h *Handler) parseProxies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	rows, err := proxyinput.Preview(req.Text, req.Type, proxySnapshot(h.Store))
	if err != nil {
		writeJSON(w, 400, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows})
}
func (h *Handler) importProxies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []config.Proxy `json:"items"`
	}
	if !decodeRequest(w, r, &req) {
		return
	}
	if len(req.Items) == 0 || len(req.Items) > proxyinput.MaxItems {
		writeJSON(w, 400, map[string]any{"detail": "每次请导入 1–200 条有效节点"})
		return
	}
	added, skipped := 0, 0
	err := h.Store.Update(func(c *config.Config) error {
		seen := map[string]bool{}
		for _, p := range c.Proxies {
			seen[config.ProxyIdentity(p)] = true
		}
		for _, p := range req.Items {
			p = config.NormalizeProxy(p)
			if err := config.ValidateProxyConfig([]config.Proxy{p}); err != nil {
				return err
			}
			key := config.ProxyIdentity(p)
			if seen[key] {
				skipped++
				continue
			}
			p.ID = "proxy_" + key[:24]
			// A collision must never overwrite an existing node or its account bindings.
			for _, old := range c.Proxies {
				if old.ID == p.ID {
					return errors.New("代理 ID 冲突，请重新导入")
				}
			}
			c.Proxies = append(c.Proxies, p)
			seen[key] = true
			added++
		}
		return validateProxyMutation(c)
	})
	if err != nil {
		writeJSON(w, 400, map[string]any{"detail": err.Error()})
		return
	}
	if added > 0 {
		h.ProxyChanged()
	}
	writeJSON(w, 200, map[string]any{"success": true, "added": added, "skipped": skipped})
}
