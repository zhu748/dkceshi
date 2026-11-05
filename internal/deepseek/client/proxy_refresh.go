package client

import "ds2api/internal/config"

// RefreshProxyClients retires changed/deleted connections and preserves pools
// for unchanged nodes, including display-name changes and account rebindings.
func (c *Client) RefreshProxyClients() {
	if c == nil {
		return
	}
	valid := map[string]bool{"direct": true}
	if c.Store != nil {
		for _, p := range c.Store.ProxySnapshot() {
			valid[proxyCacheKey(config.NormalizeProxy(p))] = true
		}
	}
	old := map[string]requestClients{}
	c.proxyClientsMu.Lock()
	c.proxyEpoch++
	for key, bundle := range c.proxyClients {
		if !valid[key] {
			old[key] = bundle
			delete(c.proxyClients, key)
		}
	}
	c.proxyClientsMu.Unlock()
	closeProxyBundles(old)
}
