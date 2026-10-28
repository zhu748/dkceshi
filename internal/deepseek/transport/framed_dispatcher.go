package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// UseFramedH2 defaults to ordered HTTP/2 on all deployments. Explicit false
// retains the legacy transport as an opt-out; it cannot preserve header order.
func UseFramedH2() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_USE_FRAMED_H2")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// FramedDoer 是一个简化接口，让 deepseek client 调用 H2FramedClient。
// 注意：H2FramedClient 本身实现了 http.RoundTripper，但因为需要通过 context
// 注入 ordered headers，所以包一层 helper 方法。
type FramedDoer interface {
	DoWithOrderedHeaders(ctx context.Context, method, url string, headers map[string]string, body []byte) (*http.Response, error)
}

// framedClientPool 缓存 H2FramedClient 实例（按 timeout 维度），避免每次请求都 new。
var framedClientPool sync.Map // map[time.Duration]*H2FramedClient

// getFramedClient 按 timeout 获取 H2FramedClient（dialer 与 NewFramedH2WithDialContext 一致）
func getFramedClient(timeout time.Duration) *H2FramedClient {
	if v, ok := framedClientPool.Load(timeout); ok {
		return v.(*H2FramedClient)
	}
	c := NewFramedH2WithDialContext(timeout, nil)
	v, _ := framedClientPool.LoadOrStore(timeout, c)
	return v.(*H2FramedClient)
}

// DoWithOrderedHeaders 是 H2FramedClient 的辅助方法，把 map headers + body
// 转成 http.Request 并通过 H2FramedClient.Do 发出。
// 内部用 BuildOrderedHeaders 构造有序 header 列表，注入 context。
func DoWithOrderedHeaders(ctx context.Context, timeout time.Duration, method, urlString string, headers map[string]string, body []byte) (*http.Response, error) {
	c := getFramedClient(timeout)
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlString, bodyReader)
	if err != nil {
		return nil, err
	}
	// 注入有序 headers
	parsed, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	ordered := BuildOrderedRequestHeaders(method, parsed.Path, headers, int64(len(body)))
	ctx = WithOrderedHeaders(req.Context(), ordered)
	req = req.WithContext(ctx)
	return c.Do(req)
}

// FramedPostJSON 是给 deepseek/client.postJSONWithStatus 用的便捷封装，
// 直接发 POST + JSON body 并返回响应。
func FramedPostJSON(ctx context.Context, timeout time.Duration, url string, headers map[string]string, body []byte) (*http.Response, error) {
	// 注入 Content-Type: application/json（若未指定）
	if _, ok := headers["Content-Type"]; !ok {
		hdrs := make(map[string]string, len(headers)+1)
		for k, v := range headers {
			hdrs[k] = v
		}
		hdrs["Content-Type"] = "application/json"
		headers = hdrs
	}
	return DoWithOrderedHeaders(ctx, timeout, http.MethodPost, url, headers, body)
}
