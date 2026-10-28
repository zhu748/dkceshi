package transport

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	tls "github.com/refraction-networking/utls"
	"golang.org/x/net/proxy"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// WebSocket opcodes (RFC 6455 §5.2).
const (
	WSOpContinuation byte = 0x0
	WSOpText         byte = 0x1
	WSOpBinary       byte = 0x2
	WSOpClose        byte = 0x8
	WSOpPing         byte = 0x9
	WSOpPong         byte = 0xA
)

const (
	// TTS audio frames are a few hundred bytes each; 4 MiB per frame and 8 MiB
	// per message leave orders of magnitude of headroom while keeping a
	// misbehaving peer from pinning memory.
	wsMaxPayload = 4 << 20
	wsMaxMessage = 8 << 20
	// wsHandshakeTimeout covers TCP + TLS + the HTTP Upgrade round trip.
	wsHandshakeTimeout = 30 * time.Second
)

// WebSocketDialOptions describes one upstream WebSocket upgrade.
type WebSocketDialOptions struct {
	Host        string
	PathQuery   string
	ProxyURL    string
	DialContext DialContextFunc
	// Origin / UserAgent / AcceptLanguage describe the TTS upgrade. Empty
	// values fall back to the configured Android client defaults.
	Origin         string
	UserAgent      string
	AcceptLanguage string
	Timeout        time.Duration
}

// WSConn is a minimal RFC 6455 client connection. It deliberately implements
// only what the DeepSeek TTS stream needs: text/binary frames, fragmented
// messages, ping/pong and close. permessage-deflate is advertised in the
// handshake (the real browser sends it) but the server does not accept it, so
// payloads are never compressed.
type WSConn struct {
	conn net.Conn
	br   *bufio.Reader

	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    bool
	closeErr  error
}

// DialWebSocket opens a Android TLS connection and performs the
// HTTP/1.1 WebSocket upgrade. It returns the connection plus the upgrade
// response (useful for surfacing non-101 errors).
func DialWebSocket(ctx context.Context, opts WebSocketDialOptions) (*WSConn, *http.Response, error) {
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		return nil, nil, errors.New("websocket host is empty")
	}
	pathQuery := strings.TrimSpace(opts.PathQuery)
	if pathQuery == "" {
		return nil, nil, errors.New("websocket path is empty")
	}
	if !strings.HasPrefix(pathQuery, "/") {
		pathQuery = "/" + pathQuery
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = wsHandshakeTimeout
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dial := opts.DialContext
	if dial == nil {
		dial = func(ctx context.Context, _, addr string) (net.Conn, error) {
			return dialWebSocketTCP(ctx, opts.ProxyURL, addr)
		}
	}
	rawConn, err := dial(dialCtx, "tcp", net.JoinHostPort(host, "443"))
	if err != nil {
		return nil, nil, err
	}

	uconn := tls.UClient(rawConn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}, defaultTLSFingerprintProfiles[0].helloID)
	if err := forceALPN(uconn, []string{"http/1.1"}); err != nil {
		closeTransportConn(rawConn)
		return nil, nil, err
	}
	if err := uconn.HandshakeContext(dialCtx); err != nil {
		closeTransportConn(rawConn)
		return nil, nil, fmt.Errorf("websocket tls handshake: %w", err)
	}
	if err := uconn.SetDeadline(time.Now().Add(timeout)); err != nil {
		closeTransportConn(uconn)
		return nil, nil, err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		closeTransportConn(uconn)
		return nil, nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)

	origin := strings.TrimSpace(opts.Origin)
	if origin == "" {
		origin = "https://" + host
	}
	userAgent := strings.TrimSpace(opts.UserAgent)
	if userAgent == "" {
		userAgent = "DeepSeek/2.7.1 Android/36"
	}
	acceptLanguage := strings.TrimSpace(opts.AcceptLanguage)
	if acceptLanguage == "" {
		acceptLanguage = "zh-CN,zh;q=0.9"
	}

	// Header order mirrors a real Chrome WebSocket upgrade captured from
	// chat.deepseek.com. Do not add Authorization/Cookie: the ticket in the
	// query string is the only credential this endpoint accepts.
	headers := []string{
		"Upgrade: websocket",
		"Origin: " + origin,
		"Cache-Control: no-cache",
		"Accept-Language: " + acceptLanguage,
		"Pragma: no-cache",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: " + key,
		"Accept-Encoding: gzip, deflate, br, zstd",
		"User-Agent: " + userAgent,
		"Sec-WebSocket-Version: 13",
		"Host: " + host,
		"Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits",
	}
	var request strings.Builder
	request.Grow(512)
	request.WriteString("GET ")
	request.WriteString(pathQuery)
	request.WriteString(" HTTP/1.1\r\n")
	for _, header := range headers {
		request.WriteString(header)
		request.WriteString("\r\n")
	}
	request.WriteString("\r\n")
	if err := writeAll(uconn, []byte(request.String())); err != nil {
		closeTransportConn(uconn)
		return nil, nil, fmt.Errorf("websocket write upgrade: %w", err)
	}

	br := bufio.NewReaderSize(uconn, 64*1024)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		closeTransportConn(uconn)
		return nil, nil, fmt.Errorf("websocket read upgrade response: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err := resp.Body.Close(); err != nil {
			slog.Warn("cleanup close failed", "error", err)
		}
		closeTransportConn(uconn)
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = resp.Status
		}
		return nil, resp, fmt.Errorf("websocket upgrade rejected: %s: %s", resp.Status, detail)
	}
	expectedAccept := base64.StdEncoding.EncodeToString(sha1Sum(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if got := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Accept")); got != expectedAccept {
		closeTransportConn(uconn)
		return nil, resp, fmt.Errorf("websocket accept mismatch: got %q want %q", got, expectedAccept)
	}
	// permessage-deflate is advertised to mirror the browser handshake, but this
	// client cannot decompress: readFrame rejects RSV bits outright. If the
	// server ever negotiates an extension, fail here instead of misreading
	// every data frame that follows.
	if ext := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Extensions")); ext != "" {
		closeTransportConn(uconn)
		return nil, resp, fmt.Errorf("websocket: server negotiated unsupported extension: %s", ext)
	}
	if err := uconn.SetDeadline(time.Time{}); err != nil {
		closeTransportConn(uconn)
		return nil, resp, err
	}
	return &WSConn{conn: uconn, br: br}, resp, nil
}

func dialWebSocketTCP(ctx context.Context, proxyURL, addr string) (net.Conn, error) {
	direct := &net.Dialer{Timeout: wsHandshakeTimeout}
	if strings.TrimSpace(proxyURL) == "" {
		return environmentProxyDialer(direct.DialContext)(ctx, "tcp", addr)
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return dialConnectTunnel(ctx, direct.DialContext, parsed, addr)
	}
	dialer, err := proxy.FromURL(parsed, direct)
	if err != nil {
		return nil, err
	}
	if d, ok := dialer.(proxy.ContextDialer); ok {
		return d.DialContext(ctx, "tcp", addr)
	}
	return nil, errors.New("websocket proxy has no context dialer")
}

func sha1Sum(value string) []byte {
	sum := sha1.Sum([]byte(value))
	return sum[:]
}

// SetReadDeadline bounds the next read; pass the zero time to clear it.
func (c *WSConn) SetReadDeadline(t time.Time) error {
	if c == nil || c.conn == nil {
		return net.ErrClosed
	}
	return c.conn.SetReadDeadline(t)
}

// WriteText sends one masked text frame.
func (c *WSConn) WriteText(payload []byte) error {
	return c.writeFrame(WSOpText, payload)
}

// WriteBinary sends one masked binary frame.
func (c *WSConn) WriteBinary(payload []byte) error {
	return c.writeFrame(WSOpBinary, payload)
}

// ReadMessage returns the next complete text/binary message. Ping frames are
// answered transparently, pongs are ignored, and a close frame surfaces as
// io.EOF after echoing the close.
func (c *WSConn) ReadMessage() (byte, []byte, error) {
	if c == nil || c.br == nil {
		return 0, nil, net.ErrClosed
	}
	var opcode byte
	var payload []byte
	inFragment := false
	for {
		fin, op, data, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch op {
		case WSOpPing:
			if err := c.writeFrame(WSOpPong, data); err != nil {
				return 0, nil, err
			}
			continue
		case WSOpPong:
			continue
		case WSOpClose:
			_ = c.writeFrame(WSOpClose, data)
			return WSOpClose, data, io.EOF
		case WSOpContinuation:
			if !inFragment {
				return 0, nil, errors.New("websocket: unexpected continuation frame")
			}
		default:
			if inFragment {
				return 0, nil, errors.New("websocket: expected continuation frame")
			}
			opcode = op
			inFragment = !fin
		}
		payload = append(payload, data...)
		if len(payload) > wsMaxMessage {
			return 0, nil, fmt.Errorf("websocket message exceeds %d bytes", wsMaxMessage)
		}
		if fin {
			return opcode, payload, nil
		}
	}
}

func (c *WSConn) readFrame() (bool, byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.br, header[:]); err != nil {
		return false, 0, nil, err
	}
	fin := header[0]&0x80 != 0
	if header[0]&0x70 != 0 {
		return false, 0, nil, errors.New("websocket: unexpected RSV bits")
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > wsMaxPayload {
		return false, 0, nil, fmt.Errorf("websocket frame exceeds %d bytes", wsMaxPayload)
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, maskKey[:]); err != nil {
			return false, 0, nil, err
		}
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return false, 0, nil, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	if opcode >= 0x8 {
		if !fin || length > 125 {
			return false, 0, nil, errors.New("websocket: invalid control frame")
		}
	}
	return fin, opcode, payload, nil
}

func (c *WSConn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return net.ErrClosed
	}

	var header [14]byte
	header[0] = 0x80 | opcode
	n := 2
	switch {
	case len(payload) < 126:
		header[1] = 0x80 | byte(len(payload))
	case len(payload) <= 0xffff:
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
		n = 4
	default:
		header[1] = 0x80 | 127
		binary.BigEndian.PutUint64(header[2:], uint64(len(payload)))
		n = 10
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	copy(header[n:], mask[:])
	n += 4

	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	frame := make([]byte, 0, n+len(masked))
	frame = append(frame, header[:n]...)
	frame = append(frame, masked...)
	return writeAll(c.conn, frame)
}

// Close sends a normal-closure frame and closes the underlying connection.
func (c *WSConn) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.conn == nil {
			return
		}
		// Cancellation must also finish when the peer has stopped reading.
		deadlineErr := c.conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
		frameErr := c.writeFrame(WSOpClose, []byte{0x03, 0xe8}) // 1000 normal closure
		c.writeMu.Lock()
		c.closed = true
		c.writeMu.Unlock()
		c.closeErr = errors.Join(deadlineErr, frameErr, c.conn.Close())
	})
	return c.closeErr
}

func writeAll(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

// NewWSConnForTest constructs a minimal WSConn wrapping conn for internal test suites.
func NewWSConnForTest(conn net.Conn) *WSConn {
	return &WSConn{conn: conn, br: bufio.NewReaderSize(conn, 64*1024)}
}
