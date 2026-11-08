package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type claudeProxyStoreStub struct {
	aliases map[string]string
}

func (s claudeProxyStoreStub) ModelAliases() map[string]string { return s.aliases }

func (claudeProxyStoreStub) CurrentInputFileEnabled() bool    { return true }
func (claudeProxyStoreStub) CurrentInputFileMinChars() int    { return 0 }
func (claudeProxyStoreStub) ExpertPromptSegmentEnabled() bool { return false }
func (claudeProxyStoreStub) ExpertPromptSegmentMaxChars() int { return 120000 }

type openAIProxyStub struct {
	status int
	body   string
}

func TestClaudeProxyViaOpenAIPrefersGlobalAliasMapping(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store: claudeProxyStoreStub{
			aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4.1-flash"},
		},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(openAI.seenModel); got != "deepseek-v4.1-flash" {
		t.Fatalf("expected global alias mapped proxy model deepseek-v4.1-flash, got %q", got)
	}
}

func (s openAIProxyStub) ChatCompletions(w http.ResponseWriter, _ *http.Request) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

type openAIProxyCaptureStub struct {
	seenModel string
	seenReq   map[string]any
}

func (s *openAIProxyCaptureStub) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.seenReq = req
	if m, ok := req["model"].(string); ok {
		s.seenModel = m
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
}

func TestClaudeProxyViaOpenAIVercelPreparePassthrough(t *testing.T) {
	h := &Handler{OpenAI: openAIProxyStub{status: 200, body: `{"lease_id":"lease_123","payload":{"a":1}}`}}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("expected json response, got err=%v body=%s", err, rec.Body.String())
	}
	if _, ok := out["lease_id"]; !ok {
		t.Fatalf("expected lease_id in prepare passthrough, got=%v", out)
	}
}

func TestClaudeProxyViaOpenAIUsesGlobalAliasMapping(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-3-opus": "deepseek-v4.1-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-3-opus","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(openAI.seenModel); got != "deepseek-v4.1-flash" {
		t.Fatalf("expected mapped proxy model deepseek-v4.1-flash, got %q", got)
	}
}

// 思考开关只由模型名决定：转发体不再注入任何 thinking 标记，内部
// OpenAI 出口按模型名自行解析。Claude 请求里的 thinking 配置仅兼容接收。
func TestClaudeProxyViaOpenAIDoesNotInjectThinkingIntoTranslatedRequest(t *testing.T) {
	for _, thinkingCfg := range []string{
		``,
		`,"thinking":{"type":"disabled"}`,
		`,"thinking":{"type":"enabled","budget_tokens":1024}`,
	} {
		openAI := &openAIProxyCaptureStub{}
		h := &Handler{
			Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4.1-flash"}},
			OpenAI: openAI,
		}
		req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}]`+thinkingCfg+`,"stream":false}`))
		rec := httptest.NewRecorder()

		h.Messages(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		if _, exists := openAI.seenReq["thinking"]; exists {
			t.Fatalf("translated request must not carry a thinking override, got %#v", openAI.seenReq["thinking"])
		}
	}
}

func TestClaudeProxyViaOpenAIDoesNotInjectThinkingIntoStreamRequest(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4.1-flash"}},
		OpenAI: openAI,
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	thinking, _ := openAI.seenReq["thinking"].(map[string]any)
	if thinking != nil {
		t.Fatalf("translated stream request must not carry a thinking override, got %#v", thinking)
	}
}

func TestClaudeProxyViaOpenAIExposesThinkingBlocksByDefault(t *testing.T) {
	body := `{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"claude-sonnet-4-5","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"internal reasoning","tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	h := &Handler{Store: claudeProxyStoreStub{}, OpenAI: openAIProxyStub{status: 200, body: body}}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, `"type":"thinking"`) {
		t.Fatalf("expected converted Claude response to expose thinking block, got %s", got)
	}
	if !strings.Contains(got, `"tool_use"`) {
		t.Fatalf("expected converted Claude response to preserve tool_use, got %s", got)
	}
}

// -nothinking 模型关闭思考：响应里的 reasoning_content 被剔除；
// 请求里的 thinking 配置无法重新打开。
func TestClaudeProxyViaOpenAIStripsThinkingBlocksForNoThinkingModel(t *testing.T) {
	body := `{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"claude-sonnet-4-5","choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_content":"internal reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	h := &Handler{
		Store:  claudeProxyStoreStub{aliases: map[string]string{"claude-sonnet-4-6": "deepseek-v4.1-flash-nothinking"}},
		OpenAI: openAIProxyStub{status: 200, body: body},
	}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","budget_tokens":1024},"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if strings.Contains(got, `"type":"thinking"`) {
		t.Fatalf("expected nothinking model to strip thinking block, got %s", got)
	}
}

func TestClaudeProxyPreservesNativeImageSourceForSharedUpload(t *testing.T) {
	openAI := &openAIProxyCaptureStub{}
	h := &Handler{OpenAI: openAI}
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJDRA=="}}]}],"stream":false}`))
	rec := httptest.NewRecorder()

	h.Messages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	messages, _ := openAI.seenReq["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected one translated message, got %#v", openAI.seenReq)
	}
	msg, _ := messages[0].(map[string]any)
	content, _ := msg["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected translated content blocks, got %#v", msg)
	}
	imageBlock, _ := content[1].(map[string]any)
	if strings.TrimSpace(asString(imageBlock["type"])) != "image" {
		t.Fatalf("expected native image block, got %#v", imageBlock)
	}
	source, _ := imageBlock["source"].(map[string]any)
	if source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != "QUJDRA==" {
		t.Fatalf("native source changed before upload validation: %#v", imageBlock)
	}
}
