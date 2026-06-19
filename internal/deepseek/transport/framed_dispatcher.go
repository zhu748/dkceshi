// Package transport - framed_dispatcher.go
//
// 把 deepseek client 的 streamPost / postJSONWithStatus 请求路由到 H2FramedClient，
// 当满足以下任一条件时启用：
//   - 显式设置环境变量 DS2API_DEEPSEEK_USE_FRAMED_H2=1/true/yes/on
//   - 运行在 Vercel 部署环境（VERCEL=1 自动注入）
//
// 显式设置 DS2API_DEEPSEEK_USE_FRAMED_H2=0/false/no/off 可强制关闭（优先级最高）。
// 其它环境（本地开发、Docker 等）默认关闭，保留 utls + net/http2 稳定路径。
//
// 开启后：所有 chat.deepseek.com 请求走自定义 H2FramedClient，可控制 header 顺序与
// HPACK never-indexed 行为，对齐真实 Android OkHttp App 抓包。
package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// useFramedH2 返回是否启用自定义 H2 Framed 客户端。
//
// 启用规则（按优先级递减）：
//  1. 显式环境变量 DS2API_DEEPSEEK_USE_FRAMED_H2：值为 "1"/"true"/"yes"/"on" 启用，
//     "0"/"false"/"no"/"off" 显式关闭。
//  2. 自动检测：当运行在 Vercel 部署环境（VERCEL=1 是 Vercel 自动注入的运行时环境变量）
//     时自动启用，对齐真实 Android OkHttp App 的字节级指纹（header 顺序、HPACK never-indexed）。
//  3. 其它环境（本地开发、Docker 等）默认关闭，保留 utls + net/http2 稳定路径。
//
// 每次调用都重新读 env，便于运行时切换（包括测试中的 t.Setenv）。
func UseFramedH2() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_USE_FRAMED_H2")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	// 未显式设置时，Vercel 部署环境自动启用
	if isVercelRuntime() {
		return true
	}
	return false
}

// isVercelRuntime 检测当前是否运行在 Vercel 部署环境。
// Vercel 会自动注入 VERCEL=1 运行时环境变量到所有 serverless function。
func isVercelRuntime() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("VERCEL")))
	return v == "1" || v == "true"
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
func DoWithOrderedHeaders(ctx context.Context, timeout time.Duration, method, url string, headers map[string]string, body []byte) (*http.Response, error) {
	c := getFramedClient(timeout)
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	// 注入有序 headers
	ordered := BuildOrderedHeaders(headers, len(body))
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
