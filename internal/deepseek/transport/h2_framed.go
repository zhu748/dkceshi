package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	fh2 "github.com/bogdanfinn/fhttp/http2"
)

type OrderedHeader struct {
	Name      string
	Value     string
	Sensitive bool
}

type orderedHeadersCtxKey struct{}

func WithOrderedHeaders(ctx context.Context, headers []OrderedHeader) context.Context {
	return context.WithValue(ctx, orderedHeadersCtxKey{}, headers)
}

// H2FramedClient preserves the HAR's header order using a maintained HTTP/2
// implementation. It handles flow control, CONTINUATION, cancellation and large
// multipart bodies; the old handwritten writer did not honor send windows.
// TLS/SETTINGS/HPACK bytes are not asserted to match the App without a raw trace.
// Idle connections are reused within one installation/token and proxy client.
type H2FramedClient struct {
	TLSConfig      *tls.Config
	DialContext    DialContextFunc
	Timeout        time.Duration
	poolMu         sync.Mutex
	idle           map[string][]*h2Session
	idleCount      int
	poolGeneration uint64
}

func (c *H2FramedClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	var cancel context.CancelFunc
	if c.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	return c.roundTrip(req, ctx, cancel)
}

func (c *H2FramedClient) connect(ctx context.Context, req *http.Request) (*h2Session, error) {
	addr := req.URL.Host
	if req.URL.Port() == "" {
		port := "443"
		if req.URL.Scheme != "https" {
			port = "80"
		}
		addr = net.JoinHostPort(req.URL.Hostname(), port)
	}
	dial := c.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme == "https" {
		if proto, ok := negotiatedProtocol(conn); ok {
			if proto != "h2" {
				return nil, errors.Join(fmt.Errorf("expected HTTP/2 ALPN, got %q", proto), conn.Close())
			}
		} else {
			cfg := c.TLSConfig
			if cfg == nil {
				cfg = &tls.Config{MinVersion: tls.VersionTLS12}
			}
			cfg = cfg.Clone()
			cfg.NextProtos = []string{"h2"}
			if cfg.ServerName == "" {
				cfg.ServerName = req.URL.Hostname()
			}
			secure := tls.Client(conn, cfg)
			if err := secure.HandshakeContext(ctx); err != nil {
				return nil, errors.Join(err, conn.Close())
			}
			if secure.ConnectionState().NegotiatedProtocol != "h2" {
				return nil, errors.Join(errors.New("expected HTTP/2 ALPN"), secure.Close())
			}
			conn = secure
		}
	}
	stop := context.AfterFunc(ctx, func() { closeTransportConn(conn) })
	defer stop()
	transport := &fh2.Transport{DisableCompression: true, PseudoHeaderOrder: []string{":method", ":authority", ":path", ":scheme"}}
	cc, err := transport.NewClientConn(conn)
	if err != nil {
		closeTransportConn(conn)
		return nil, err
	}
	return &h2Session{conn: conn, client: cc}, nil
}

func (c *H2FramedClient) roundTrip(req *http.Request, ctx context.Context, cancel context.CancelFunc) (*http.Response, error) {
	forkReq, err := fhttp.NewRequestWithContext(ctx, req.Method, req.URL.String(), req.Body)
	if err != nil {
		cancel()
		return nil, err
	}
	forkReq.ContentLength = req.ContentLength
	forkReq.GetBody = req.GetBody
	forkReq.Host = req.Host
	headers, _ := req.Context().Value(orderedHeadersCtxKey{}).([]OrderedHeader)
	if len(headers) == 0 {
		values := make(map[string]string, len(req.Header))
		for name := range req.Header {
			values[name] = req.Header.Get(name)
		}
		headers = BuildOrderedRequestHeaders(req.Method, req.URL.Path, values, req.ContentLength)
	}
	order := make([]string, 0, len(headers))
	for _, h := range headers {
		forkReq.Header[h.Name] = []string{h.Value}
		order = append(order, h.Name)
		if strings.EqualFold(h.Name, "content-type") && strings.HasPrefix(strings.ToLower(h.Value), "multipart/") {
			// The HTTP/2 library can synthesize Content-Length from this field
			// even after the header was removed. Keep App uploads stream-sized.
			forkReq.ContentLength = -1
		}
	}
	forkReq.Header[fhttp.HeaderOrderKey] = order
	forkReq.Header[fhttp.PHeaderOrderKey] = []string{":method", ":authority", ":path", ":scheme"}
	key := h2IdentityKey(req, headers)
	session, generation := c.takeIdle(key)
	if session == nil {
		session, err = c.connect(ctx, req)
		if err != nil {
			cancel()
			return nil, err
		}
	}
	stop := context.AfterFunc(ctx, func() { closeTransportConn(session.conn) })
	cleanup := func(reusable bool) {
		stopped := stop()
		healthy := reusable && stopped && ctx.Err() == nil
		cancel()
		c.returnIdle(key, session, generation, healthy)
	}
	upstream, err := session.client.RoundTrip(forkReq)
	if err != nil {
		cleanup(false)
		return nil, err
	}
	return &http.Response{
		Status: upstream.Status, StatusCode: upstream.StatusCode,
		Proto: upstream.Proto, ProtoMajor: upstream.ProtoMajor, ProtoMinor: upstream.ProtoMinor,
		Header: http.Header(upstream.Header), Trailer: http.Header(upstream.Trailer),
		Body:          &orderedResponseBody{ReadCloser: upstream.Body, cleanup: cleanup},
		ContentLength: upstream.ContentLength, Request: req, Uncompressed: upstream.Uncompressed,
	}, nil
}
