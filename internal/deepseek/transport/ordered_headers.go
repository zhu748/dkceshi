package transport

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// This order is observable in the three Android 2.7.1 HAR exports. HPACK
// indexing and HTTP/2 SETTINGS cannot be inferred from an HTTP-level export.
var appHeaderOrder = []string{
	"x-ds-pow-response", "x-thinking-enabled", "x-file-size", "x-model-type",
	"x-client-platform", "x-client-version", "x-client-locale", "x-client-bundle-id",
	"x-client-timezone-offset", "x-device-model", "x-rangers-id", "x-device-id",
	"x-settings-token", "user-agent", "authorization", "accept", "accept-charset",
	"content-type", "content-length", "accept-encoding",
}

func BuildOrderedHeaders(headers map[string]string, bodyLen int) []OrderedHeader {
	return BuildOrderedRequestHeaders(http.MethodPost, "", headers, int64(bodyLen))
}

func BuildOrderedRequestHeaders(method, path string, headers map[string]string, bodyLen int64) []OrderedHeader {
	// Sort original keys first so case collisions have a deterministic resolution.
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
	isMultipart := strings.HasPrefix(strings.ToLower(normalized["content-type"]), "multipart/")
	if method == http.MethodGet || method == http.MethodHead {
		delete(normalized, "content-type")
		delete(normalized, "content-length")
	} else if isMultipart || bodyLen < 0 {
		delete(normalized, "content-length")
	} else {
		normalized["content-length"] = strconv.FormatInt(bodyLen, 10)
	}
	if normalized["accept-encoding"] == "" {
		normalized["accept-encoding"] = "gzip"
	}
	order := appHeaderOrder
	if strings.SplitN(path, "?", 2)[0] == "/api/v0/users/auth_token/check_device" {
		order = append([]string{"authorization"}, appHeaderOrder...)
	}
	out := make([]OrderedHeader, 0, len(normalized))
	appendHeader := func(name string) {
		value, ok := normalized[name]
		if !ok {
			return
		}
		out = append(out, OrderedHeader{Name: name, Value: value, Sensitive: name == "authorization" || name == "x-ds-pow-response" || name == "x-settings-token"})
		delete(normalized, name)
	}
	for _, name := range order {
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
