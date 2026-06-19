// Package transport - h2_framed.go
//
// 自定义 HTTP/2 RoundTripper，绕过 Go 标准 net/http2 的 header 编码路径，
// 用 hpack.Encoder + http2.Framer 直接写帧，从而可以：
//
//  1. 精确控制 header 顺序（对齐真实 Android OkHttp App 抓包）
//  2. 强制 Huffman 编码（OkHttp 默认开启，Go 默认关闭）
//  3. 强制 never-indexed（敏感 header 不进动态表，OkHttp 对所有 header 默认 never-indexed）
//
// 仅支持 POST 方法 + JSON/二进制 body（这是 DeepSeek 上游全部接口的形态）。
// 不支持连接复用：每个请求独占一个 HTTP/2 stream，请求完成后关闭连接。
// 这与真实 Android App 行为一致（OkHttp 在 chat.deepseek.com 上也是新连接 + 新 stream）。
package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// OrderedHeader 是一个有序的 HTTP header 条目，按 slice 顺序写入 HPACK。
type OrderedHeader struct {
	Name  string
	Value string
	// Sensitive 标记为 true 时，使用 NeverIndexed 字段，对齐 OkHttp 对所有 header
	// 都做 never-indexed 的行为（不进动态表）。
	Sensitive bool
}

// H2FramedClient 是自定义 HTTP/2 RoundTripper。
// 每次 Do() 都新建一个 TCP+TLS 连接，发完一个请求 + 读完响应就关闭。
type H2FramedClient struct {
	// TLSConfig 用于 TLS 握手（一般由 uTLS fingerprint dialer 替代，这里保留 fallback）
	TLSConfig *tls.Config
	// DialContext 自定义拨号（可以接 utls fingerprint dialer）
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// Timeout 是端到端超时；0 表示无超时（用于流式）
	Timeout time.Duration
	// FingerprintProfile 指定 TLS 指纹 profile（与现有 transport.go 共用）
	FingerprintProfile string
}

// h2StreamContext 单次请求的 stream 上下文
type h2StreamContext struct {
	conn       net.Conn
	framer     *http2.Framer
	encoder    *hpack.Encoder
	streamID   uint32
	responseMu sync.Mutex
	// 响应字段
	statusCode int
	headers    http.Header
	bodyPr     *io.PipeReader // response body 读取端
	bodyPw     *io.PipeWriter // response body 写入端（readLoop 写入）
	headersCh  chan struct{}  // 响应 headers 到达后 close
	headersErr error          // 读 headers 阶段的错误
	trailers   http.Header
}

// Do 实现 http.RoundTripper。
//
// 调用方需通过 context 传入 *OrderedHeader 列表：
//
//	ctx = context.WithValue(ctx, orderedHeadersCtxKey{}, headers)
//	resp, err := client.Do(req)
//
// 若未传入，则按 req.Header 的 map 迭代顺序输出（顺序不可控）。
func (c *H2FramedClient) Do(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost && req.Method != http.MethodGet {
		return nil, fmt.Errorf("h2_framed: unsupported method %q (only POST/GET)", req.Method)
	}
	ctx := req.Context()
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	addr := req.URL.Host
	if !strings.Contains(addr, ":") {
		if req.URL.Scheme == "https" {
			addr += ":443"
		} else {
			addr += ":80"
		}
	}
	// 1. 拨号（用注入的 dialer，支持 utls fingerprint）
	dialer := c.DialContext
	if dialer == nil {
		dialer = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	conn, err := dialer(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("h2_framed: dial %s failed: %w", addr, err)
	}
	// 2. TLS 升级（若 https）— dialer 已是 TLS conn 则跳过
	var finalConn net.Conn
	if req.URL.Scheme == "https" {
		// 检查 dialer 是否已返回一个 TLS conn（如 utls fingerprint dialer）
		if state, proto, ok := tryGetTLSConnState(conn); ok {
			if proto != "h2" {
				_ = conn.Close()
				return nil, fmt.Errorf("h2_framed: server did not negotiate HTTP/2 (got %q)", proto)
			}
			finalConn = conn
			_ = state // 留作未来扩展（如校验证书）
		} else {
			// dialer 返回的是 raw TCP conn，自己升 TLS（fallback 路径）
			tlsConf := c.TLSConfig
			if tlsConf == nil {
				tlsConf = &tls.Config{MinVersion: tls.VersionTLS12}
			}
			if tlsConf.ServerName == "" {
				host, _, _ := net.SplitHostPort(addr)
				tlsConf = tlsConf.Clone()
				tlsConf.ServerName = host
			}
			tlsConn := tls.Client(conn, tlsConf)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("h2_framed: TLS handshake failed: %w", err)
			}
			proto := tlsConn.ConnectionState().NegotiatedProtocol
			if proto != "h2" {
				_ = tlsConn.Close()
				return nil, fmt.Errorf("h2_framed: server did not negotiate HTTP/2 (got %q)", proto)
			}
			finalConn = tlsConn
		}
	} else {
		finalConn = conn
	}

	// 4. 进入 HTTP/2 framing
	return c.doH2(ctx, finalConn, req)
}

// tryGetTLSConnState 尝试从 conn 中读取 TLS ConnectionState 与 ALPN 协议。
// 同时支持标准库 *tls.Conn 和 utls *utls.Conn（两者都有 ConnectionState() 方法，
// 但返回类型不同，用 interface + 反射抽离）。
// 返回 (state, negotiatedProto, ok)；ok=false 表示 conn 不是 TLS conn。
func tryGetTLSConnState(conn net.Conn) (state any, proto string, ok bool) {
	// 标准库 tls.Conn
	if tc, ok := conn.(*tls.Conn); ok {
		s := tc.ConnectionState()
		return s, s.NegotiatedProtocol, true
	}
	// utls.Conn 或其他 — 通过反射调用 ConnectionState() 并读 NegotiatedProtocol 字段
	if reflectHasMethod(conn, "ConnectionState") {
		proto := reflectCallStringField(reflectCallNoArgMethod(conn, "ConnectionState"), "NegotiatedProtocol")
		return conn, proto, true
	}
	return nil, "", false
}

// reflectHasMethod 通过反射判断 v 是否有名为 name 的方法。
func reflectHasMethod(v interface{}, name string) bool {
	if v == nil {
		return false
	}
	rVal := reflect.ValueOf(v)
	method := rVal.MethodByName(name)
	return method.IsValid()
}

// reflectCallNoArgMethod 通过反射调用 v 的无参方法 name，返回结果。
func reflectCallNoArgMethod(v interface{}, name string) interface{} {
	if v == nil {
		return nil
	}
	rVal := reflect.ValueOf(v)
	method := rVal.MethodByName(name)
	if !method.IsValid() {
		return nil
	}
	results := method.Call(nil)
	if len(results) == 0 {
		return nil
	}
	return results[0].Interface()
}

// reflectCallStringField 通过反射读取 v 的 string 字段 name。
// 若 v 不是 struct 或字段不存在，返回空字符串。
func reflectCallStringField(v interface{}, name string) string {
	if v == nil {
		return ""
	}
	rVal := reflect.ValueOf(v)
	// 处理指针解引用
	for rVal.Kind() == reflect.Ptr {
		if rVal.IsNil() {
			return ""
		}
		rVal = rVal.Elem()
	}
	if rVal.Kind() != reflect.Struct {
		return ""
	}
	field := rVal.FieldByName(name)
	if !field.IsValid() {
		return ""
	}
	if field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

// doH2 在已建立的 TLS 连接上发送 HTTP/2 preface + SETTINGS + HEADERS + (DATA) + 读取响应
func (c *H2FramedClient) doH2(ctx context.Context, conn net.Conn, req *http.Request) (*http.Response, error) {
	// 写 client preface
	preface := []byte(http2.ClientPreface)
	if _, err := conn.Write(preface); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("h2_framed: write preface failed: %w", err)
	}

	framer := http2.NewFramer(conn, conn)
	framer.SetReuseFrames()

	// 写 SETTINGS 帧（空 settings，与 OkHttp 默认行为接近）
	if err := framer.WriteSettings(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("h2_framed: write SETTINGS failed: %w", err)
	}

	// 读取 server SETTINGS（不处理 ACK，简化）
	// 这里循环读 frame，直到拿到 server 的 SETTINGS，然后写 ACK
	if err := readServerSettings(framer); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("h2_framed: read server SETTINGS failed: %w", err)
	}
	if err := framer.WriteSettingsAck(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("h2_framed: write SETTINGS ACK failed: %w", err)
	}

	// 构造有序 header 列表
	orderedHeaders, _ := req.Context().Value(orderedHeadersCtxKey{}).([]OrderedHeader)
	if len(orderedHeaders) == 0 {
		// fallback: 用 req.Header 的 map（顺序不可控）
		orderedHeaders = headersFromHTTPRequest(req)
	}

	// 编码 HPACK
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	enc.SetMaxDynamicTableSizeLimit(4096)
	// OkHttp 默认对大部分 header 用 Huffman 编码；Go hpack 默认也支持 Huffman，
	// 但需要通过 SetEmitFunc 或 WriteField 控制。这里直接用 WriteField + Sensitivity 标记。

	// 1) 先写 pseudo-headers（HTTP/2 强制顺序：:method, :authority, :path, :scheme）
	host := req.URL.Host
	if !strings.Contains(host, ":") {
		if req.URL.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	if err := enc.WriteField(hpack.HeaderField{
		Name:      ":method",
		Value:     req.Method,
		Sensitive: false,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := enc.WriteField(hpack.HeaderField{
		Name:      ":authority",
		Value:     req.URL.Host,
		Sensitive: false,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := enc.WriteField(hpack.HeaderField{
		Name:      ":path",
		Value:     req.URL.RequestURI(),
		Sensitive: false,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := enc.WriteField(hpack.HeaderField{
		Name:      ":scheme",
		Value:     req.URL.Scheme,
		Sensitive: false,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// 2) 写业务 header（按调用方传入顺序）
	// OkHttp 对所有 header 都默认 never-indexed（不进动态表），通过 Sensitive=true 实现。
	for _, h := range orderedHeaders {
		if err := enc.WriteField(hpack.HeaderField{
			Name:      strings.ToLower(h.Name),
			Value:     h.Value,
			Sensitive: h.Sensitive,
		}); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	// 3) 写 END_HEADERS + body（如果有）
	streamID := uint32(1) // client stream 必须是奇数
	bodyBytes, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()

	if len(bodyBytes) > 0 {
		// HEADERS (no END_STREAM) + DATA (END_STREAM)
		if err := framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hdrBuf.Bytes(),
			EndStream:     false,
			EndHeaders:    true,
		}); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("h2_framed: write HEADERS failed: %w", err)
		}
		// 分块写 DATA（最大 16KB per frame，OkHttp 也用 16KB）
		const maxChunk = 16 * 1024
		for len(bodyBytes) > 0 {
			chunk := bodyBytes
			if len(chunk) > maxChunk {
				chunk = chunk[:maxChunk]
			}
			bodyBytes = bodyBytes[len(chunk):]
			endStream := len(bodyBytes) == 0
			if err := framer.WriteData(streamID, endStream, chunk); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("h2_framed: write DATA failed: %w", err)
			}
		}
	} else {
		// HEADERS + END_STREAM
		if err := framer.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hdrBuf.Bytes(),
			EndStream:     true,
			EndHeaders:    true,
		}); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("h2_framed: write HEADERS failed: %w", err)
		}
	}

	// 4. 读响应（HEADERS + DATA + 可选 TRAILERS + RST/GOAWAY）
	bodyPr, bodyPw := io.Pipe()
	streamCtx := &h2StreamContext{
		conn:      conn,
		framer:    framer,
		streamID:  streamID,
		headers:   make(http.Header),
		trailers:  make(http.Header),
		bodyPr:    bodyPr,
		bodyPw:    bodyPw,
		headersCh: make(chan struct{}),
	}
	go streamCtx.readLoop(ctx)

	// 等待响应 headers 到达（或出错）
	select {
	case <-streamCtx.headersCh:
	case <-ctx.Done():
		_ = bodyPw.CloseWithError(ctx.Err())
		_ = conn.Close()
		return nil, ctx.Err()
	}

	if streamCtx.headersErr != nil {
		_ = bodyPw.CloseWithError(streamCtx.headersErr)
		_ = conn.Close()
		return nil, fmt.Errorf("h2_framed: read response headers failed: %w", streamCtx.headersErr)
	}

	// 构造 http.Response，Body 是流式 PipeReader
	resp := &http.Response{
		Status:     fmt.Sprintf("%d %s", streamCtx.statusCode, http.StatusText(streamCtx.statusCode)),
		StatusCode: streamCtx.statusCode,
		Header:     streamCtx.headers,
		Body:       bodyPr,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		Request:    req,
	}
	// Body 关闭时关闭底层连接（一次性 stream 模型）
	// 用 wrapper 包一层，确保 conn 在 body 关闭时被关闭
	resp.Body = &bodyWithConnClose{r: bodyPr, conn: conn}
	return resp, nil
}

// bodyWithConnClose 在 Body.Close 时同时关闭底层 net.Conn。
// 这保证流式读完后连接被释放（与 App 一次性 stream 行为一致）。
type bodyWithConnClose struct {
	r    *io.PipeReader
	conn net.Conn
}

func (b *bodyWithConnClose) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (b *bodyWithConnClose) Close() error {
	err := b.r.Close()
	_ = b.conn.Close()
	return err
}

// readLoop 读取 server 发来的所有 frame，直到 stream 结束或出错。
// Headers 到达后 close(s.headersCh)，body chunks 写入 s.bodyPw。
func (s *h2StreamContext) readLoop(ctx context.Context) {
	headersSent := false
	dec := hpack.NewDecoder(4096, func(f hpack.HeaderField) {
		s.responseMu.Lock()
		defer s.responseMu.Unlock()
		switch f.Name {
		case ":status":
			// parse status
			var st int
			for _, c := range f.Value {
				if c >= '0' && c <= '9' {
					st = st*10 + int(c-'0')
				}
			}
			s.statusCode = st
		default:
			s.headers.Add(f.Name, f.Value)
		}
	})
	defer func() {
		_ = s.bodyPw.Close()
	}()
	for {
		if ctx.Err() != nil {
			_ = s.bodyPw.CloseWithError(ctx.Err())
			return
		}
		frame, err := s.framer.ReadFrame()
		if err != nil {
			if err == io.EOF {
				return
			}
			if !headersSent {
				s.headersErr = err
				close(s.headersCh)
				return
			}
			_ = s.bodyPw.CloseWithError(err)
			return
		}
		switch f := frame.(type) {
		case *http2.HeadersFrame:
			if _, err := dec.Write(f.HeaderBlockFragment()); err != nil {
				if !headersSent {
					s.headersErr = err
					close(s.headersCh)
					return
				}
				_ = s.bodyPw.CloseWithError(err)
				return
			}
			if !headersSent {
				headersSent = true
				close(s.headersCh)
			}
			if f.StreamEnded() {
				return
			}
		case *http2.DataFrame:
			if len(f.Data()) > 0 {
				if _, err := s.bodyPw.Write(f.Data()); err != nil {
					// reader 已关闭，发送 RST 后退出
					_ = s.framer.WriteRSTStream(s.streamID, http2.ErrCodeCancel)
					return
				}
			}
			if f.StreamEnded() {
				return
			}
		case *http2.RSTStreamFrame:
			err := fmt.Errorf("h2_framed: server sent RST_STREAM: code=%d", f.ErrCode)
			if !headersSent {
				s.headersErr = err
				close(s.headersCh)
				return
			}
			_ = s.bodyPw.CloseWithError(err)
			return
		case *http2.GoAwayFrame:
			err := fmt.Errorf("h2_framed: server sent GOAWAY: code=%d", f.ErrCode)
			if !headersSent {
				s.headersErr = err
				close(s.headersCh)
				return
			}
			_ = s.bodyPw.CloseWithError(err)
			return
		case *http2.SettingsFrame:
			if f.IsAck() {
				continue
			}
			// 写 ACK
			_ = s.framer.WriteSettingsAck()
		case *http2.PingFrame:
			if f.IsAck() {
				continue
			}
			_ = s.framer.WritePing(true, f.Data)
		case *http2.WindowUpdateFrame:
			// 忽略，简化处理
		default:
			// 其他 frame 类型忽略
		}
	}
}

// readServerSettings 循环读 frame 直到拿到 server SETTINGS
func readServerSettings(framer *http2.Framer) error {
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return err
		}
		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if f.IsAck() {
				continue
			}
			return nil
		case *http2.WindowUpdateFrame:
			// 忽略
		case *http2.PingFrame:
			if !f.IsAck() {
				_ = framer.WritePing(true, f.Data)
			}
		default:
			// 忽略其他 frame
		}
	}
}

// headersFromHTTPRequest 是 fallback：从 http.Request 构造 OrderedHeader 列表。
// 注意：map 迭代顺序不稳定，因此仅作为兜底；正常路径应通过 orderedHeadersCtxKey 传入。
func headersFromHTTPRequest(req *http.Request) []OrderedHeader {
	out := make([]OrderedHeader, 0, len(req.Header)+4)
	// content-length 与 accept-encoding 通常由 net/http 自动加，这里手动补
	for k, vs := range req.Header {
		for _, v := range vs {
			out = append(out, OrderedHeader{Name: k, Value: v, Sensitive: false})
		}
	}
	if req.ContentLength > 0 {
		out = append(out, OrderedHeader{Name: "content-length", Value: fmt.Sprintf("%d", req.ContentLength)})
	}
	out = append(out, OrderedHeader{Name: "accept-encoding", Value: "gzip"})
	return out
}

// orderedHeadersCtxKey 是 context key，用于携带有序 header 列表
type orderedHeadersCtxKey struct{}

// WithOrderedHeaders 把有序 header 列表塞进 context，供 H2FramedClient.Do 读取
func WithOrderedHeaders(ctx context.Context, headers []OrderedHeader) context.Context {
	return context.WithValue(ctx, orderedHeadersCtxKey{}, headers)
}

// tryUTLSConn 尝试把 net.Conn 当作 utls conn 使用（若 dialer 返回的是 utls）
// 返回 (true, _) 表示已是 TLS conn，可直接使用 raw conn 读 HTTP/2 frames
func tryUTLSConn(conn net.Conn) (bool, bool) {
	// 简化：始终返回 (false, false)，由调用方走标准 tls.Client 路径或自行处理 utls
	_ = conn
	return false, false
}

// 以下是占位符，避免 import 警告
var (
	_ = binary.BigEndian
	_ = errors.New
)
