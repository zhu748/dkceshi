package shared

import (
	"strings"

	"ds2api/internal/config"
)

func ToProxy(m map[string]any) config.Proxy {
	return config.NormalizeProxy(config.Proxy{
		ID:         FieldString(m, "id"),
		Name:       FieldString(m, "name"),
		Type:       FieldString(m, "type"),
		Host:       FieldString(m, "host"),
		Port:       IntFrom(m["port"]),
		Username:   rawProxyString(m, "username"),
		Password:   rawProxyString(m, "password"),
		ServerName: FieldString(m, "tls_server_name"), CAFile: FieldString(m, "ca_file"),
		Insecure: m["insecure"] == true, Realm: rawProxyString(m, "realm"), MTU: IntFrom(m["mtu"]),
	})
}

func FindProxyByID(c config.Config, proxyID string) (config.Proxy, bool) {
	id := strings.TrimSpace(proxyID)
	if id == "" {
		return config.Proxy{}, false
	}
	for _, proxy := range c.Proxies {
		proxy = config.NormalizeProxy(proxy)
		if proxy.ID == id {
			return proxy, true
		}
	}
	return config.Proxy{}, false
}

func rawProxyString(m map[string]any, key string) string { v, _ := m[key].(string); return v }
