package client

import (
	trans "ds2api/internal/deepseek/transport"
	"net/http"
)

func (c *Client) ResetProxyClients() {
	c.proxyClientsMu.Lock()
	old := c.proxyClients
	c.proxyClients = make(map[string]requestClients)
	c.proxyClientsMu.Unlock()
	for _, bundle := range old {
		for _, d := range []trans.Doer{bundle.regular, bundle.stream, bundle.fallback, bundle.fallbackS} {
			if closer, ok := d.(interface{ CloseIdleConnections() }); ok {
				closer.CloseIdleConnections()
			}
		}
	}
}

func (c *Client) SetNodeFailureReporter(fn func(string, bool)) {
	c.nodeReporterMu.Lock()
	c.nodeReporter = fn
	c.nodeReporterMu.Unlock()
}

type reportingDoer struct {
	inner  trans.Doer
	id     string
	report func(string, bool)
}

func (d reportingDoer) Do(r *http.Request) (*http.Response, error) {
	resp, err := d.inner.Do(r)
	d.report(d.id, err == nil)
	return resp, err
}

func (d reportingDoer) CloseIdleConnections() {
	if c, ok := d.inner.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func (c *Client) reportingBundle(id string, b requestClients) requestClients {
	c.nodeReporterMu.RLock()
	report := c.nodeReporter
	c.nodeReporterMu.RUnlock()
	if id == "" || report == nil {
		return b
	}
	b.regular = reportingDoer{b.regular, id, report}
	b.stream = reportingDoer{b.stream, id, report}
	if b.fallback != nil {
		b.fallback = reportingDoer{b.fallback, id, report}
	}
	if b.fallbackS != nil {
		b.fallbackS = reportingDoer{b.fallbackS, id, report}
	}
	return b
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.ResetProxyClients()
	for _, d := range []trans.Doer{c.regular, c.stream, c.fallback, c.fallbackS} {
		if closer, ok := d.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}
