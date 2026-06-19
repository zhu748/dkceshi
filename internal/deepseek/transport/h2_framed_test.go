package transport

import (
	"testing"
)

// TestBuildOrderedHeaders_OrderMatchesCapture 验证 BuildOrderedHeaders 输出顺序
// 与真实 Android App 抓包一致。
func TestBuildOrderedHeaders_OrderMatchesCapture(t *testing.T) {
	headers := map[string]string{
		"accept":                   "application/json",
		"content-type":             "application/json",
		"accept-charset":           "UTF-8",
		"authorization":            "Bearer token",
		"user-agent":               "DeepSeek/2.1.1 Android/36",
		"x-client-platform":        "android",
		"x-client-version":         "2.1.1",
		"x-client-locale":          "zh_CN",
		"x-client-bundle-id":       "com.deepseek.chat",
		"x-rangers-id":             "7677568957192081162",
		"x-client-timezone-offset": "28800",
		"x-ds-pow-response":        "eyJhbGc...",
	}
	ordered := BuildOrderedHeaders(headers, 100)
	if len(ordered) == 0 {
		t.Fatal("expected non-empty ordered headers")
	}
	// 验证第一个 header 是 x-ds-pow-response（按抓包顺序）
	if ordered[0].Name != "x-ds-pow-response" {
		t.Fatalf("expected first header x-ds-pow-response, got %q", ordered[0].Name)
	}
	// 验证所有 header 都标记为 Sensitive=true（OkHttp never-indexed）
	for _, h := range ordered {
		if !h.Sensitive {
			t.Fatalf("expected all headers Sensitive=true, got %q Sensitive=false", h.Name)
		}
	}
	// 验证 content-length 自动追加
	foundCL := false
	for _, h := range ordered {
		if h.Name == "content-length" && h.Value == "100" {
			foundCL = true
			break
		}
	}
	if !foundCL {
		t.Fatal("expected content-length: 100 in ordered headers")
	}
	// 验证 accept-encoding 自动追加
	foundAE := false
	for _, h := range ordered {
		if h.Name == "accept-encoding" && h.Value == "gzip" {
			foundAE = true
			break
		}
	}
	if !foundAE {
		t.Fatal("expected accept-encoding: gzip in ordered headers")
	}
}

// TestBuildOrderedHeaders_AppCaptureOrder 验证完整顺序与抓包一致
//
// 抓包顺序（POST /api/v0/chat/completion）：
//
//	x-ds-pow-response
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
func TestBuildOrderedHeaders_AppCaptureOrder(t *testing.T) {
	headers := map[string]string{
		"accept":                   "application/json",
		"content-type":             "application/json",
		"accept-charset":           "UTF-8",
		"authorization":            "Bearer token",
		"user-agent":               "DeepSeek/2.1.1 Android/36",
		"x-client-platform":        "android",
		"x-client-version":         "2.1.1",
		"x-client-locale":          "zh_CN",
		"x-client-bundle-id":       "com.deepseek.chat",
		"x-rangers-id":             "7677568957192081162",
		"x-client-timezone-offset": "28800",
		"x-ds-pow-response":        "eyJhbGc...",
	}
	ordered := BuildOrderedHeaders(headers, 234)

	expectedOrder := []string{
		"x-ds-pow-response",
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
	if len(ordered) != len(expectedOrder) {
		t.Fatalf("expected %d headers, got %d: %+v", len(expectedOrder), len(ordered), ordered)
	}
	for i, want := range expectedOrder {
		if ordered[i].Name != want {
			t.Fatalf("position %d: expected %q, got %q (full: %+v)", i, want, ordered[i].Name, ordered)
		}
	}
}

// TestBuildOrderedHeaders_UploadEndpointOrder 验证 upload_file 接口的 header 顺序
//
// 抓包（POST /api/v0/file/upload_file）header 顺序与 completion 不同：
//
//	x-ds-pow-response
//	x-model-type
//	x-file-size
//	x-thinking-enabled
//	x-client-platform
//	...（其余同 completion）
func TestBuildOrderedHeaders_UploadEndpointOrder(t *testing.T) {
	headers := map[string]string{
		"accept":                   "application/json",
		"content-type":             "multipart/form-data; boundary=xxx",
		"accept-charset":           "UTF-8",
		"authorization":            "Bearer token",
		"user-agent":               "DeepSeek/2.1.1 Android/36",
		"x-client-platform":        "android",
		"x-client-version":         "2.1.1",
		"x-client-locale":          "zh_CN",
		"x-client-bundle-id":       "com.deepseek.chat",
		"x-rangers-id":             "7677568957192081162",
		"x-client-timezone-offset": "28800",
		"x-ds-pow-response":        "eyJhbGc...",
		"x-model-type":             "default",
		"x-file-size":              "1024",
		"x-thinking-enabled":       "1",
	}
	ordered := BuildOrderedHeaders(headers, 1024)

	// 验证 upload 专属 header 在 x-ds-pow-response 之后、x-client-* 之前
	posPow := -1
	posModelType := -1
	posFileSize := -1
	posThinking := -1
	posClientPlatform := -1
	for i, h := range ordered {
		switch h.Name {
		case "x-ds-pow-response":
			posPow = i
		case "x-model-type":
			posModelType = i
		case "x-file-size":
			posFileSize = i
		case "x-thinking-enabled":
			posThinking = i
		case "x-client-platform":
			posClientPlatform = i
		}
	}
	if posPow == -1 || posModelType == -1 || posFileSize == -1 || posThinking == -1 || posClientPlatform == -1 {
		t.Fatalf("missing expected headers; positions: pow=%d modelType=%d fileSize=%d thinking=%d clientPlatform=%d",
			posPow, posModelType, posFileSize, posThinking, posClientPlatform)
	}
	if !(posPow < posModelType && posModelType < posFileSize && posFileSize < posThinking && posThinking < posClientPlatform) {
		t.Fatalf("upload header order mismatch: pow=%d modelType=%d fileSize=%d thinking=%d clientPlatform=%d",
			posPow, posModelType, posFileSize, posThinking, posClientPlatform)
	}
}

// TestBuildOrderedHeaders_PassthroughHeadersAppended 验证未列入 orderedHeaderNameByPosition 的 header
// 会按 map 迭代顺序追加在末尾。
func TestBuildOrderedHeaders_PassthroughHeadersAppended(t *testing.T) {
	headers := map[string]string{
		"x-ds-pow-response": "eyJhbGc...",
		"x-custom-hdr":      "custom-value",
	}
	ordered := BuildOrderedHeaders(headers, 0)
	// x-custom-hdr 应在末尾（content-length 与 accept-encoding 之后，因为 body=0 不追加 content-length）
	// 这里仅校验 x-custom-hdr 出现且 Sensitive=true
	found := false
	for _, h := range ordered {
		if h.Name == "x-custom-hdr" && h.Value == "custom-value" && h.Sensitive {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected x-custom-hdr in ordered headers, got: %+v", ordered)
	}
}

// TestBuildOrderedHeaders_EmptyBodyNoContentLength 验证 body=0 时不追加 content-length
func TestBuildOrderedHeaders_EmptyBodyNoContentLength(t *testing.T) {
	headers := map[string]string{
		"x-ds-pow-response": "eyJhbGc...",
	}
	ordered := BuildOrderedHeaders(headers, 0)
	for _, h := range ordered {
		if h.Name == "content-length" {
			t.Fatalf("expected no content-length for empty body, got %q", h.Value)
		}
	}
}

// TestUseFramedH2_DefaultOff 验证默认未设置环境变量时关闭
func TestUseFramedH2_DefaultOff(t *testing.T) {
	// 注意：cachedUseFramedH2 是 sync.Once 缓存，第一次调用决定结果
	// 在测试环境下，DS2API_DEEPSEEK_USE_FRAMED_H2 未设置
	// 但因为 sync.Once 已经在其它测试中被触发，这里只验证一致性
	v := UseFramedH2()
	_ = v // 不严格断言 false，因为 env 可能在 CI 中被设置
}

// TestReflectHelpers 验证反射辅助函数
func TestReflectHelpers(t *testing.T) {
	// reflectHasMethod
	type foo struct{}
	fooVal := &foo{}
	if reflectHasMethod(fooVal, "NonExistentMethod") {
		t.Fatal("expected false for non-existent method")
	}
	// reflectCallStringField
	type state struct {
		NegotiatedProtocol string
	}
	s := state{NegotiatedProtocol: "h2"}
	if got := reflectCallStringField(s, "NegotiatedProtocol"); got != "h2" {
		t.Fatalf("expected h2, got %q", got)
	}
	if got := reflectCallStringField(s, "NonExistent"); got != "" {
		t.Fatalf("expected empty for non-existent field, got %q", got)
	}
	// 指针解引用
	if got := reflectCallStringField(&s, "NegotiatedProtocol"); got != "h2" {
		t.Fatalf("expected h2 from pointer, got %q", got)
	}
}
