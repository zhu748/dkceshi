package client

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/devid"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// sdkReportBundle assembles everything one SDK report submission needs: the
// reporter closure, its release hook, and the Chrome major the browser env
// must claim so the advertised generation matches the ClientHello this
// transport will actually dial.
type sdkReportBundle struct {
	report      devid.Reporter
	release     func()
	chromeMajor string
}

// SDK collection has browser headers, not App credentials or App TLS presets.
// It shares only the account's egress selection with normal upstream requests.
//
// The submit path prefers the httpcloak transport (trans.NewSDKReportCloakClient,
// chrome-150-android: TLS ClientHello, HTTP/2 frame fingerprint and Chrome
// header order all from one captured preset) and sends Chrome-ordered headers:
// the report claims a Chrome UA plus the sec-ch-ua trio, so a Go-stdlib
// handshake or an Android-okHttp handshake behind those headers is an
// immediate non-browser tell the moment the SDK service inspects anything
// below the HTTP layer.
//
// Accounts whose egress httpcloak cannot dial (socks4/socks4a/sstp/turn/turns
// proxies, or an env proxy of an unsupported scheme) keep the legacy uTLS
// Chrome transport (trans.NewSDKReportClient) and claim the older major that
// hello actually reproduces — a fingerprint generation behind, but never a
// request whose UA and handshake disagree.
func (c *Client) deviceReporter(acc config.Account) (sdkReportBundle, error) {
	if c.clientsClosed() {
		return sdkReportBundle{}, errClientClosed
	}
	proxyCfg, proxied := c.resolveProxyForAccount(acc)
	if !proxied && acc.ProxyID != "" && !acc.NoProxy {
		return sdkReportBundle{}, errors.New("devid: account proxy is unavailable")
	}
	if proxied {
		if proxyURL, ok := httpCloakProxyURL(proxyCfg); ok {
			client := trans.NewSDKReportCloakClient(sdkReportTimeout, proxyURL)
			return sdkReportBundle{
				report:      sdkReportFunc(client),
				release:     client.CloseIdleConnections,
				chromeMajor: trans.SDKReportChromeMajor(),
			}, nil
		}
		// socks4/socks4a/sstp/turn/turns: httpcloak cannot dial these; keep the
		// legacy uTLS path so the report at least rides a Chrome ClientHello
		// through the account's own egress.
		dial, err := proxyDialContext(proxyCfg)
		if err != nil {
			return sdkReportBundle{}, err
		}
		client := trans.NewSDKReportClient(sdkReportTimeout, dial)
		return sdkReportBundle{
			report:      sdkReportFunc(client),
			release:     client.CloseIdleConnections,
			chromeMajor: trans.LegacySDKReportChromeMajor(),
		}, nil
	}
	// No account proxy. acc.NoProxy means "no proxy at all, environment
	// included"; otherwise an env proxy the deployment already relies on stays
	// honored when httpcloak can dial it, and the legacy env-aware transport
	// takes over when it cannot.
	if !acc.NoProxy {
		envURL, envOK, envPresent := cloakEnvProxy()
		if envPresent && !envOK {
			client := trans.NewSDKReportClient(sdkReportTimeout, nil)
			return sdkReportBundle{
				report:      sdkReportFunc(client),
				release:     client.CloseIdleConnections,
				chromeMajor: trans.LegacySDKReportChromeMajor(),
			}, nil
		}
		if envOK {
			client := trans.NewSDKReportCloakClient(sdkReportTimeout, envURL)
			return sdkReportBundle{
				report:      sdkReportFunc(client),
				release:     client.CloseIdleConnections,
				chromeMajor: trans.SDKReportChromeMajor(),
			}, nil
		}
	}
	client := trans.NewSDKReportCloakClient(sdkReportTimeout, "")
	return sdkReportBundle{
		report:      sdkReportFunc(client),
		release:     client.CloseIdleConnections,
		chromeMajor: trans.SDKReportChromeMajor(),
	}, nil
}

// sdkReportTimeout bounds one SDK report submission (WAF challenge and report
// upload included); generation itself has its own larger budget upstream.
const sdkReportTimeout = 60 * time.Second

// sdkReportFunc builds the Reporter closure over the chosen doer. The recipe
// comes from the SDK environment; forward exactly the browser corpus it is
// allowed to emit, nothing else. The ordered-headers context is consumed by
// the legacy framed H2 transport and ignored by httpcloak, which applies the
// preset's own header order — harmless to set either way.
func sdkReportFunc(doer trans.Doer) devid.Reporter {
	return func(ctx context.Context, report devid.ReportRequest) (int, []byte, error) {
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
		// Chrome's own header order for a fetch/XHR POST, plus the
		// transport-level defaults (content-length, accept-encoding) a
		// browser would attach that the JS recipe cannot observe.
		req = req.WithContext(trans.WithOrderedHeaders(req.Context(),
			trans.BuildSDKReportOrderedHeaders(report.Headers, len(report.Body))))
		resp, err := doer.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer closeMetadataBody(resp)
		body, err := readSDKReportBody(resp)
		if err != nil {
			return resp.StatusCode, nil, err
		}
		return resp.StatusCode, body, nil
	}
}

// readSDKReportBody decodes the (possibly compressed) SDK service response
// under the same 1 MiB ceiling ValidateReport applies to the request payload.
// httpcloak responses arrive already decoded (their Content-Encoding metadata
// is stripped by the transport conversion), so the switch simply falls through
// for that path.
func readSDKReportBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "zstd":
		zr, err := zstd.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
	}
	data, err := io.ReadAll(io.LimitReader(reader, 1<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("devid: SDK response exceeds size limit")
	}
	return data, nil
}
