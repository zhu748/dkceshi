package transport

import (
	"context"
	"crypto/tls"
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

func New(timeout time.Duration) *Client {
	return NewWithDialContext(timeout, nil)
}

func NewWithDialContext(timeout time.Duration, dialContext DialContextFunc) *Client {
	useEnvProxy := dialContext == nil
	if dialContext == nil {
		dialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	base := &http.Transport{
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DialContext:         dialContext,
		DialTLSContext:      fingerprintTLSDialer(dialContext, http1FingerprintProfiles()),
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if useEnvProxy {
		base.Proxy = http.ProxyFromEnvironment
	}
	orderedDial := dialContext
	if useEnvProxy {
		orderedDial = environmentProxyDialer(orderedDial)
	}
	return &Client{http: &http.Client{Timeout: timeout, Transport: base}, ordered: NewFramedH2WithDialContext(timeout, orderedDial)}
}

// NewFramedH2WithDialContext preserves observable HTTP/2 header ordering.
// The Android TLS preset is a compatibility baseline, not measured from HAR.
func NewFramedH2WithDialContext(timeout time.Duration, dialContext DialContextFunc) *H2FramedClient {
	if dialContext == nil {
		dialContext = environmentProxyDialer((&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext)
	}
	// 用 utls fingerprint dialer 包装 raw dialer，得到已握手的 utls conn
	fpDialer := fingerprintTLSDialer(dialContext, defaultTLSFingerprintProfiles)
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
func http1FingerprintProfiles() []fingerprintProfile {
	profiles := append([]fingerprintProfile(nil), defaultTLSFingerprintProfiles...)
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
			if err := uConn.HandshakeContext(ctx); err != nil {
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
	if len(errs) == 0 {
		return fmt.Errorf("tls fingerprint dial failed for %s", addr)
	}
	msg := "tls fingerprint dial failed for " + addr
	for _, err := range errs {
		if err == nil {
			continue
		}
		msg += "; " + err.Error()
	}
	return fmt.Errorf("%s", msg)
}
