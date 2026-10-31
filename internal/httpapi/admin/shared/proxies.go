package shared

import (
	"strings"

	"ds2api/internal/config"
)

func ToProxy(m map[string]any) config.Proxy {
	return config.NormalizeProxy(config.Proxy{
		ID:       FieldString(m, "id"),
		Name:     FieldString(m, "name"),
		Type:     FieldString(m, "type"),
		Host:     FieldString(m, "host"),
		Port:     IntFrom(m["port"]),
		Username: FieldString(m, "username"),
		Password: FieldString(m, "password"),
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
