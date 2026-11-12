package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type DialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type Client struct {
	http    *http.Client
	ordered *H2FramedClient
}

type fingerprintProfile struct {
	name          string
	helloID       utls.ClientHelloID
	alpnProtocols []string
}

var defaultTLSFingerprintProfiles = []fingerprintProfile{
	{name: "android-okhttp", helloID: utls.HelloAndroid_11_OkHttp, alpnProtocols: []string{"h2", "http/1.1"}},
}

// sdkReportTLSFingerprintProfiles carries the Chrome ClientHello used for SDK
// fingerprint reporting on the legacy uTLS transport. The report carries
// browser-style headers (Chrome UA, sec-ch-ua trio, Origin/Referer/Sec-Fetch-*),
// so the handshake must look like a Chrome handshake too: a Go stdlib handshake
// behind those headers is an immediate non-browser tell, and a fleet of accounts
// sharing that single stdlib ClientHello is a stronger clustering signal than
// the payload itself.
//
// This is now the compatibility path: it serves accounts whose egress proxy
// cannot be expressed to httpcloak (socks4/socks4a/sstp/turn/turns) or whose
// deployment relies on an env proxy of an unsupported scheme. Accounts on
// this path claim the major this hello actually reproduces
// (LegacySDKReportChromeMajor) instead of the httpcloak generation, so the
// request stays internally coherent even though the hello itself is older.
//
// HelloChrome_Auto tracks the newest Chrome ClientHello the bundled uTLS
// models (currently Chrome 133); the legacy env is told to claim the same
// major via LegacySDKReportChromeMajor so UA, sec-ch-ua, brands and TLS never
// disagree inside one request.
var sdkReportTLSFingerprintProfiles = []fingerprintProfile{
	{name: "chrome-browser", helloID: utls.HelloChrome_Auto, alpnProtocols: []string{"h2", "http/1.1"}},
}

// LegacySDKReportChromeMajor returns the Chrome major the legacy uTLS SDK
// report path's ClientHello actually reproduces. Accounts routed onto that
// path (exotic proxy types httpcloak cannot dial) claim this version so the
// advertised UA generation and the on-wire handshake stay coherent. Keep it
// the single source the devid runner consults for those accounts.
func LegacySDKReportChromeMajor() string {
	if v := sdkReportTLSFingerprintProfiles[0].helloID.Version; v != "" {
		return v
	}
	return "133"
}

func New(timeout time.Duration) *Client {
	return NewWithDialContext(timeout, nil)
}

func NewWithDialContext(timeout time.Duration, dialContext DialContextFunc) *Client {
	return newFingerprintClient(timeout, dialContext, defaultTLSFingerprintProfiles)
}

// NewSDKReportClient builds the legacy uTLS client used for SDK fingerprint
// report submissions when the account's egress cannot ride the httpcloak
// transport (socks4/socks4a/sstp/turn/turns proxies, or an incompatible env
// proxy). It is NewWithDialContext with the Chrome browser profile instead of
// the Android-okHttp one: H2 framed (ordered headers) as primary, HTTP/1.1
// with the same Chrome ClientHello as fallback. Both paths dial with a Chrome
// handshake, so a transport-level fallback cannot downgrade the report back
// into a Go-fingerprinted connection.
//
// Prefer NewSDKReportCloakClient wherever the dial policy can be expressed to
// httpcloak: it carries the current chrome-150-android generation (TLS, H2
// framing, header order) instead of this older captured hello.
func NewSDKReportClient(timeout time.Duration, dialContext DialContextFunc) *Client {
	return newFingerprintClient(timeout, dialContext, sdkReportTLSFingerprintProfiles)
}

func newFingerprintClient(timeout time.Duration, dialContext DialContextFunc, profiles []fingerprintProfile) *Client {
	useEnvProxy := dialContext == nil
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if len(profiles) == 0 {
		profiles = defaultTLSFingerprintProfiles
	}
	base := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
		DialContext:         dialContext,
		DialTLSContext:      fingerprintTLSDialer(dialContext, http1FingerprintProfiles(profiles)),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if useEnvProxy {
		base.Proxy = http.ProxyFromEnvironment
	}
	orderedDial := dialContext
	if useEnvProxy {
		orderedDial = environmentProxyDialer(orderedDial)
	}
	return &Client{http: &http.Client{Timeout: timeout, Transport: base}, ordered: NewFramedH2WithDialContextAndProfiles(timeout, orderedDial, profiles)}
}

// NewFramedH2WithDialContext preserves observable HTTP/2 header ordering.
// The Android TLS preset is a compatibility baseline, not measured from HAR.
func NewFramedH2WithDialContext(timeout time.Duration, dialContext DialContextFunc) *H2FramedClient {
	return NewFramedH2WithDialContextAndProfiles(timeout, dialContext, defaultTLSFingerprintProfiles)
}

// NewFramedH2WithDialContextAndProfiles is NewFramedH2WithDialContext with an
// explicit ClientHello profile list, letting the SDK report path keep its
// Chrome handshake on the framed transport instead of the Android-okHttp one.
func NewFramedH2WithDialContextAndProfiles(timeout time.Duration, dialContext DialContextFunc, profiles []fingerprintProfile) *H2FramedClient {
	if dialContext == nil {
		dialContext = environmentProxyDialer((&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext)
	}
	if len(profiles) == 0 {
		profiles = defaultTLSFingerprintProfiles
	}
	// 用 utls fingerprint dialer 包装 raw dialer，得到已握手的 utls conn
	fpDialer := fingerprintTLSDialer(dialContext, profiles)
	return &H2FramedClient{Timeout: timeout, DialContext: fpDialer}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if UseFramedH2() && req.URL.Scheme == "https" {
		return c.ordered.Do(req)
	}
	return c.http.Do(req)
}

func (c *Client) CloseIdleConnections() {
	if c != nil && c.ordered != nil {
		c.ordered.CloseIdleConnections()
	}
	if c != nil && c.http != nil {
		c.http.CloseIdleConnections()
	}
}

// net/http does not recognize a uTLS connection as crypto/tls.Conn. Keep its
// legacy transport on HTTP/1.1; the framed transport owns HTTP/2 negotiation.
func http1FingerprintProfiles(profiles []fingerprintProfile) []fingerprintProfile {
	if len(profiles) == 0 {
		profiles = defaultTLSFingerprintProfiles
	}
	profiles = append([]fingerprintProfile(nil), profiles...)
	for i := range profiles {
		profiles[i].alpnProtocols = []string{"http/1.1"}
	}
	return profiles
}

func NewFallbackClient(timeout time.Duration, dialContext DialContextFunc) *http.Client {
	useEnvProxy := dialContext == nil
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	base := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
		DialContext:         dialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if useEnvProxy {
		base.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Timeout: timeout, Transport: base}
}

func fingerprintTLSDialer(dialContext DialContextFunc, profiles []fingerprintProfile) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if len(profiles) == 0 {
		profiles = defaultTLSFingerprintProfiles
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		var errs []error
		for _, profile := range profiles {
			plainConn, err := dialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			uCfg := &utls.Config{ServerName: host}
			uConn := utls.UClient(plainConn, uCfg, profile.helloID)
			if err := forceALPN(uConn, profile.alpnProtocols); err != nil {
				closeTransportConn(plainConn)
				errs = append(errs, fmt.Errorf("%s build handshake: %w", profile.name, err))
				continue
			}
			handshakeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err = uConn.HandshakeContext(handshakeCtx)
			cancel()
			if err != nil {
				closeTransportConn(plainConn)
				errs = append(errs, fmt.Errorf("%s handshake: %w", profile.name, err))
				continue
			}
			negotiated := uConn.ConnectionState().NegotiatedProtocol
			if !allowedNegotiatedALPN(negotiated, profile.alpnProtocols) {
				closeTransportConn(uConn)
				errs = append(errs, fmt.Errorf("%s negotiated unexpected ALPN protocol: %s", profile.name, negotiated))
				continue
			}
			return uConn, nil
		}
		return nil, joinDialErrors(addr, errs)
	}
}

func forceALPN(uConn *utls.UConn, protocols []string) error {
	if len(protocols) == 0 {
		protocols = []string{"http/1.1"}
	}
	if err := uConn.BuildHandshakeState(); err != nil {
		return err
	}
	for _, ext := range uConn.Extensions {
		alpnExt, ok := ext.(*utls.ALPNExtension)
		if !ok {
			continue
		}
		alpnExt.AlpnProtocols = append([]string(nil), protocols...)
		return uConn.BuildHandshakeState()
	}
	uConn.Extensions = append(uConn.Extensions, &utls.ALPNExtension{AlpnProtocols: append([]string(nil), protocols...)})
	return uConn.BuildHandshakeState()
}

func allowedNegotiatedALPN(negotiated string, allowed []string) bool {
	if negotiated == "" {
		return true
	}
	for _, candidate := range allowed {
		if negotiated == candidate {
			return true
		}
	}
	return false
}

func joinDialErrors(addr string, errs []error) error {
	joined := errors.Join(errs...)
	if joined == nil {
		return fmt.Errorf("tls fingerprint dial failed for %s", addr)
	}
	return fmt.Errorf("tls fingerprint dial failed for %s: %w", addr, joined)
}
