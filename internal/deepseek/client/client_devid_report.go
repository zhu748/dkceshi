package client

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/devid"
)

// SDK collection has browser headers, not App credentials or App TLS presets.
// It shares only the account's egress selection with normal upstream requests.
func (c *Client) deviceReporter(acc config.Account) (devid.Reporter, func(), error) {
	var dial trans.DialContextFunc
	if p, ok := c.resolveProxyForAccount(acc); ok {
		var err error
		dial, err = proxyDialContext(p)
		if err != nil {
			return nil, nil, err
		}
	} else if acc.ProxyID != "" && !acc.NoProxy {
		return nil, nil, errors.New("devid: account proxy is unavailable")
	} else if acc.NoProxy {
		dial = (&net.Dialer{Timeout: 15 * time.Second}).DialContext
	}
	client := trans.NewFallbackClient(60*time.Second, dial)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	report := func(ctx context.Context, report devid.ReportRequest) (int, []byte, error) {
		if err := devid.ValidateReport(report); err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, report.URL, strings.NewReader(report.Body))
		if err != nil {
			return 0, nil, err
		}
		headers := make(map[string]string, len(report.Headers))
		for key, value := range report.Headers {
			headers[strings.ToLower(key)] = value
		}
		for _, key := range []string{"Accept", "Accept-Language", "Content-Type", "Origin", "Referer", "Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "User-Agent", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"} {
			if value := headers[strings.ToLower(key)]; value != "" {
				req.Header.Set(key, value)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer closeMetadataBody(resp)
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
		if len(body) > 1<<20 {
			return resp.StatusCode, nil, errors.New("devid: SDK response exceeds size limit")
		}
		return resp.StatusCode, body, err
	}
	return report, client.CloseIdleConnections, nil
}
