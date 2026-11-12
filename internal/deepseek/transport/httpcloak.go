package transport

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	httpcloak "github.com/sardanioss/httpcloak/client"
)

// mergeHeaderOrder overlays a DeepSeek-specific order on top of a browser
// preset. Headers listed by customOrder keep their requested order; headers
// present only in baseOrder are appended in their original order. Ported from
// ds2api so x-ds-pow-response & friends stay pinned without depending on
// httpcloak's preset internals.
func mergeHeaderOrder(baseOrder, customOrder []string) []string {
	seen := make(map[string]struct{}, len(baseOrder)+len(customOrder))
	out := make([]string, 0, len(baseOrder)+len(customOrder))
	appendUnique := func(values []string) {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	appendUnique(customOrder)
	appendUnique(baseOrder)
	return out
}

// httpCloakDoer adapts httpcloak's client to the Doer interface. The SDK
// report is a single POST with a bounded body, so unlike ds2api's full port
// this variant carries no streaming/SSE path; the framed H2 and websocket
// transports elsewhere in this package keep their own implementations.
type httpCloakDoer struct {
	client *httpcloak.Client
}

// newSDKReportCloakClient builds the httpcloak client used for SDK fingerprint
// report submissions. It dials with the ChromePresetName browser profile
// (chrome-150-android: TLS ClientHello, HTTP/2 frame fingerprint, default
// browser headers) and pins Chrome's header order on top.
//
// Protocol negotiation is auto (H2 preferred, HTTP/1.1 fallback on H2-level
// failure) under the same preset fingerprint — this mirrors ds2api's
// newHTTPCloakFallbackDoer, the transport its devidHTTPClient uses for the
// same SDK endpoint — because fp-it-acc.portal101.cn, unlike
// chat.deepseek.com, has no captured protocol shape to force H2 against.
// DisableHTTP3 keeps auto mode from reaching for QUIC.
func newSDKReportCloakClient(timeout time.Duration, proxyURL string) *httpCloakDoer {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	opts := []httpcloak.Option{
		httpcloak.WithTimeout(timeout),
		httpcloak.WithTLSOnly(),
		httpcloak.WithDisableHTTP3(),
	}
	if proxyURL != "" {
		opts = append(opts, httpcloak.WithTCPProxy(proxyURL))
	}
	client := httpcloak.NewClient(ChromePresetName, opts...)
	client.SetHeaderOrder(mergeHeaderOrder(nil, chromeHeaderOrder))
	return &httpCloakDoer{client: client}
}

// NewSDKReportCloakClient returns the httpcloak-based SDK report doer. proxyURL
// must be a scheme httpcloak can dial (http/https/socks5) or empty for a
// direct connection; the caller (deviceReporter) is responsible for routing
// exotic proxy types onto the legacy uTLS transport instead.
func NewSDKReportCloakClient(timeout time.Duration, proxyURL string) *httpCloakDoer {
	return newSDKReportCloakClient(timeout, proxyURL)
}

func (d *httpCloakDoer) Do(req *http.Request) (*http.Response, error) {
	if d == nil || d.client == nil {
		return nil, fmt.Errorf("httpcloak client is nil")
	}
	resp, err := d.client.Do(req.Context(), toHTTPCloakRequest(req))
	if err != nil {
		return nil, err
	}
	return fromHTTPCloakResponse(resp, req)
}

// CloseIdleConnections releases pooled httpcloak connections when a reporter
// bundle is retired.
func (d *httpCloakDoer) CloseIdleConnections() {
	if d != nil && d.client != nil {
		d.client.Close()
	}
}

func toHTTPCloakRequest(req *http.Request) *httpcloak.Request {
	headers := make(map[string][]string, len(req.Header))
	for key, values := range req.Header {
		headers[key] = append([]string(nil), values...)
	}
	return &httpcloak.Request{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headers,
		Body:    req.Body,
	}
}

func fromHTTPCloakResponse(resp *httpcloak.Response, req *http.Request) (*http.Response, error) {
	if resp == nil {
		return nil, fmt.Errorf("httpcloak returned nil response")
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode)),
		StatusCode:    resp.StatusCode,
		Header:        normalizedHTTPCloakHeaders(resp.Headers),
		Body:          resp.Body,
		ContentLength: -1,
		Uncompressed:  true,
		Request:       req,
	}, nil
}

func httpCloakHeaders(headers map[string][]string) http.Header {
	out := make(http.Header, len(headers))
	for key, values := range headers {
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}

// httpcloak transparently decodes advertised content encodings, but retains
// the original Content-Encoding/Content-Length metadata. readSDKReportBody
// performs its own decompression step based on those headers, so forwarding
// them would make it decode an already-decoded body a second time. Ported
// verbatim from ds2api.
func normalizedHTTPCloakHeaders(headers map[string][]string) http.Header {
	out := httpCloakHeaders(headers)
	out.Del("Content-Encoding")
	out.Del("Content-Length")
	return out
}

var _ Doer = (*httpCloakDoer)(nil)
