package transport

import (
	"sort"
	"strconv"
	"strings"
)

// browserReportHeaderOrder mirrors the order a real Chrome sends for a
// cross-origin XHR/fetch POST, restricted to the headers an SDK report
// actually carries. Chrome's low-entropy Client-Hints trio rides right after
// content-length, browser identity headers follow, and the sec-fetch-* plus
// referer close the request. The relative order matches the capture behind
// the upstream web client; keep it in step when the SDK report preset moves.
var browserReportHeaderOrder = []string{
	"content-length",
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"sec-ch-ua-platform",
	"user-agent",
	"content-type",
	"accept",
	"origin",
	"sec-fetch-site",
	"sec-fetch-mode",
	"sec-fetch-dest",
	"referer",
	"accept-encoding",
	"accept-language",
	"priority",
}

// BuildSDKReportOrderedHeaders renders browser-ordered headers for one SDK
// fingerprint report request. Header values come from the SDK environment's
// own recipe; this only normalizes names, fills the transport-level defaults
// Chrome would attach (content-length, accept-encoding) and orders them the
// way Chrome does. Hop-by-hop and pseudo headers are dropped. Unknown headers
// are appended deterministically after the known ones, matching how the
// framed transport keeps custom keys on a stable tail.
func BuildSDKReportOrderedHeaders(headers map[string]string, bodyLen int) []OrderedHeader {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	normalized := make(map[string]string, len(headers)+2)
	for _, key := range keys {
		name := strings.ToLower(strings.TrimSpace(key))
		if strings.HasPrefix(name, ":") {
			continue
		}
		switch name {
		case "", "host", "connection", "proxy-connection", "transfer-encoding", "upgrade", "keep-alive":
			continue
		}
		if value := strings.TrimSpace(headers[key]); value != "" {
			normalized[name] = value
		}
	}
	if bodyLen >= 0 {
		normalized["content-length"] = strconv.Itoa(bodyLen)
	}
	// Chrome advertises the full modern set on every fetch/XHR and the
	// response decoders handle gzip/br/zstd that may come back.
	if normalized["accept-encoding"] == "" {
		normalized["accept-encoding"] = "gzip, deflate, br, zstd"
	}
	out := make([]OrderedHeader, 0, len(normalized))
	appendHeader := func(name string) {
		value, ok := normalized[name]
		if !ok {
			return
		}
		out = append(out, OrderedHeader{Name: name, Value: value})
		delete(normalized, name)
	}
	for _, name := range browserReportHeaderOrder {
		appendHeader(name)
	}
	keys = keys[:0]
	for name := range normalized {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		appendHeader(name)
	}
	return out
}
