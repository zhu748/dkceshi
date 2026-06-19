// Package transport - ordered_headers.go
//
// 把 DeepSeek 上游请求头按真实 Android App 抓包观察到的顺序，
// 转换为 []OrderedHeader，配合 H2FramedClient 实现字节级对齐。
//
// 抓包顺序（POST /api/v0/chat/completion 为例）：
//
//	:method
//	:authority
//	:path
//	:scheme
//	x-ds-pow-response        (仅 completion / upload 接口)
//	x-client-platform
//	x-client-version
//	x-client-locale
//	x-client-bundle-id
//	x-rangers-id
//	x-client-timezone-offset
//	user-agent
//	authorization
//	accept
//	accept-charset
//	content-type
//	content-length
//	accept-encoding
//
// 注意：所有 header 都标记为 Sensitive=true（never-indexed），
// 与 OkHttp 4.x 默认行为对齐（不进动态表）。
package transport

import (
	"fmt"
	"strings"
)

// orderedHeaderNameByPosition 是抓包观察到的 header 顺序（按重要性递减）。
// 调用方传入的 map 中，未列在此处的 header 会按 map 字典序追加在末尾。
var orderedHeaderNameByPosition = []string{
	"x-ds-pow-response",
	"x-model-type",
	"x-file-size",
	"x-thinking-enabled",
	"x-client-platform",
	"x-client-version",
	"x-client-locale",
	"x-client-bundle-id",
	"x-rangers-id",
	"x-client-timezone-offset",
	"user-agent",
	"authorization",
	"accept",
	"accept-charset",
	"content-type",
	"content-length",
	"accept-encoding",
}

// BuildOrderedHeaders 从 map[string]string 构造按抓包顺序排序的 OrderedHeader 列表。
//
// 行为：
//  1. 按 orderedHeaderNameByPosition 顺序输出（小写匹配，case-insensitive）
//  2. 未列出的 header 按 map 迭代顺序追加在末尾（顺序不稳定，但极少出现）
//  3. 所有 header 标记 Sensitive=true（对齐 OkHttp never-indexed 行为）
//  4. 自动追加 content-length（若未在 map 中提供且 bodyLen > 0）
//  5. 自动追加 accept-encoding: gzip
func BuildOrderedHeaders(headers map[string]string, bodyLen int) []OrderedHeader {
	out := make([]OrderedHeader, 0, len(headers)+2)
	consumed := make(map[string]bool, len(headers))
	// 1. 按抓包顺序输出
	for _, name := range orderedHeaderNameByPosition {
		for k, v := range headers {
			kLower := strings.ToLower(strings.TrimSpace(k))
			if consumed[k] {
				continue
			}
			if kLower == name {
				if strings.TrimSpace(v) == "" {
					consumed[k] = true
					break
				}
				out = append(out, OrderedHeader{
					Name:      kLower,
					Value:     v,
					Sensitive: true, // OkHttp never-indexed
				})
				consumed[k] = true
				break
			}
		}
	}
	// 2. 追加 content-length（若未提供）
	// 真实 Android App（OkHttp）即使 body 为空也会显式发送 content-length: 0，
	// 例如 chat_session/create 接口。这里对齐 App 行为：bodyLen==0 时也补 0。
	hasContentLength := false
	for _, h := range out {
		if h.Name == "content-length" {
			hasContentLength = true
			break
		}
	}
	if !hasContentLength {
		out = append(out, OrderedHeader{
			Name:      "content-length",
			Value:     fmt.Sprintf("%d", bodyLen),
			Sensitive: true,
		})
	}
	// 3. 追加 accept-encoding（OkHttp 默认 gzip）
	hasAcceptEncoding := false
	for _, h := range out {
		if h.Name == "accept-encoding" {
			hasAcceptEncoding = true
			break
		}
	}
	if !hasAcceptEncoding {
		out = append(out, OrderedHeader{
			Name:      "accept-encoding",
			Value:     "gzip",
			Sensitive: true,
		})
	}
	// 4. 追加未列入 orderedHeaderNameByPosition 的 header（如 passthrough 字段）
	for k, v := range headers {
		if consumed[k] {
			continue
		}
		kLower := strings.ToLower(strings.TrimSpace(k))
		if strings.TrimSpace(v) == "" {
			continue
		}
		out = append(out, OrderedHeader{
			Name:      kLower,
			Value:     v,
			Sensitive: true,
		})
	}
	return out
}
