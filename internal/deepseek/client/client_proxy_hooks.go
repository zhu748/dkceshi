package client

import (
	"context"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/proxybridge"
	"errors"
	"net/http"
)

func (c *Client) ResetProxyClients() {
	if c == nil {
		return
	}
	c.proxyClientsMu.Lock()
	c.proxyEpoch++
	old := c.proxyClients
	c.proxyClients = make(map[string]requestClients)
	c.proxyClientsMu.Unlock()
	closeProxyBundles(old)
}
func closeProxyBundles(items map[string]requestClients) {
	for _, bundle := range items {
		for _, d := range []trans.Doer{bundle.regular, bundle.stream, bundle.fallback, bundle.fallbackS} {
			if closer, ok := d.(interface{ CloseIdleConnections() }); ok {
				closer.CloseIdleConnections()
			}
		}
	}
}

var errClientClosed = errors.New("上游客户端已关闭")

func (c *Client) clientsClosed() bool {
	if c == nil {
		return true
	}
	c.proxyClientsMu.RLock()
	defer c.proxyClientsMu.RUnlock()
	return c.proxyClosed
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
	if err == nil || (!errors.Is(err, context.Canceled) && r.Context().Err() == nil) {
		d.report(d.id, err == nil)
	}
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
	c.proxyClientsMu.Lock()
	if c.proxyClosed {
		c.proxyClientsMu.Unlock()
		return
	}
	c.proxyClosed = true
	c.proxyEpoch++
	old := c.proxyClients
	c.proxyClients = nil
	c.proxyClientsMu.Unlock()
	closeProxyBundles(old)
	proxybridge.CloseAll()
	for _, d := range []trans.Doer{c.regular, c.stream, c.fallback, c.fallbackS} {
		if closer, ok := d.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}
