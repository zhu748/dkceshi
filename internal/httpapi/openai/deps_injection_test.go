package openai

import (
        "strings"
        "testing"

        "ds2api/internal/promptcompat"
)

type mockOpenAIConfig struct {
        aliases             map[string]string
        autoDeleteMode      string
        toolMode            string
        earlyEmit           string
        responsesTTL        int
        embedProv           string
        currentInputEnabled bool
        currentInputMin     int
        thinkingInjection   *bool
        thinkingPrompt      string
}

func (m mockOpenAIConfig) ModelAliases() map[string]string     { return m.aliases }
func (m mockOpenAIConfig) ToolcallMode() string                { return m.toolMode }
func (m mockOpenAIConfig) ToolcallEarlyEmitConfidence() string { return m.earlyEmit }
func (m mockOpenAIConfig) ResponsesStoreTTLSeconds() int       { return m.responsesTTL }
func (m mockOpenAIConfig) EmbeddingsProvider() string          { return m.embedProv }
func (m mockOpenAIConfig) AutoDeleteMode() string {
        if m.autoDeleteMode == "" {
                return "none"
        }
        return m.autoDeleteMode
}
func (m mockOpenAIConfig) AutoDeleteSessions() bool      { return false }
func (m mockOpenAIConfig) CurrentInputFileEnabled() bool { return m.currentInputEnabled }
func (m mockOpenAIConfig) CurrentInputFileMinChars() int {
        return m.currentInputMin
}
func (m mockOpenAIConfig) ThinkingInjectionEnabled() bool {
        if m.thinkingInjection == nil {
                return false
        }
        return *m.thinkingInjection
}
func (m mockOpenAIConfig) ThinkingInjectionPrompt() string     { return m.thinkingPrompt }
func (mockOpenAIConfig) ExpertPromptSegmentEnabled() bool      { return false }
func (mockOpenAIConfig) ExpertPromptSegmentMaxChars() int      { return 120000 }
func (mockOpenAIConfig) ExpertTextFileInlineEnabled() bool     { return false }
func (mockOpenAIConfig) ExpertTextFileInlineMaxFileBytes() int { return 3 * 1024 * 1024 }

func TestNormalizeOpenAIChatRequestWithConfigInterface(t *testing.T) {
        cfg := mockOpenAIConfig{
                aliases: map[string]string{
                        "my-model": "deepseek-v4.1-flash-search",
                },
        }
        req := map[string]any{
                "model":    "my-model",
                "messages": []any{map[string]any{"role": "user", "content": "hello"}},
        }
        out, err := promptcompat.NormalizeOpenAIChatRequest(cfg, req, "")
        if err != nil {
                t.Fatalf("promptcompat.NormalizeOpenAIChatRequest error: %v", err)
        }
        if out.ResolvedModel != "deepseek-v4.1-flash-search" {
                t.Fatalf("resolved model mismatch: got=%q", out.ResolvedModel)
        }
        if !out.Search || !out.Thinking {
                t.Fatalf("unexpected model flags: thinking=%v search=%v", out.Thinking, out.Search)
        }
}

func TestNormalizeOpenAIChatRequestDisablesThinkingForNoThinkingModel(t *testing.T) {
        cfg := mockOpenAIConfig{}
        req := map[string]any{
                "model":            "deepseek-v4.1-flash-nothinking",
                "messages":         []any{map[string]any{"role": "user", "content": "hello"}},
                "reasoning_effort": "high",
        }
        out, err := promptcompat.NormalizeOpenAIChatRequest(cfg, req, "")
        if err != nil {
                t.Fatalf("promptcompat.NormalizeOpenAIChatRequest error: %v", err)
        }
        if out.ResolvedModel != "deepseek-v4.1-flash-nothinking" {
                t.Fatalf("resolved model mismatch: got=%q", out.ResolvedModel)
        }
        if out.Thinking {
                t.Fatalf("expected nothinking model to force thinking off")
        }
        if out.Search {
                t.Fatalf("expected search=false for deepseek-v4.1-flash-nothinking, got=%v", out.Search)
        }
}

// 思考开关只由模型名决定：无后缀模型即使请求携带关闭思考的参数
// （thinking / reasoning / reasoning_effort / extra_body）也保持开启，
// 这些字段仅兼容接收，不再折叠成 thinking_enabled 布尔。
func TestNormalizeOpenAIChatRequestThinkingFollowsModelNameOnly(t *testing.T) {
        for _, extra := range []map[string]any{
                {"thinking": false},
                {"reasoning_effort": "none"},
                {"reasoning": map[string]any{"effort": "none"}},
                {"extra_body": map[string]any{"reasoning_effort": "none"}},
        } {
                req := map[string]any{
                        "model":    "deepseek-v4.1-flash",
                        "messages": []any{map[string]any{"role": "user", "content": "hello"}},
                }
                for k, v := range extra {
                        req[k] = v
                }
                out, err := promptcompat.NormalizeOpenAIChatRequest(mockOpenAIConfig{}, req, "")
                if err != nil {
                        t.Fatalf("promptcompat.NormalizeOpenAIChatRequest error: %v", err)
                }
                if !out.Thinking {
                        t.Fatalf("expected thinking to follow the model name (enabled), got disabled for request extra=%#v", extra)
                }
        }
}

func TestNormalizeOpenAIResponsesRequestAlwaysAcceptsWideInput(t *testing.T) {
        req := map[string]any{
                "model": "deepseek-v4.1-flash",
                "input": "hi",
        }

        out, err := promptcompat.NormalizeOpenAIResponsesRequest(mockOpenAIConfig{
                aliases: map[string]string{},
        }, req, "")
        if err != nil {
                t.Fatalf("unexpected error for wide input request: %v", err)
        }
        if out.Surface != "openai_responses" {
                t.Fatalf("unexpected surface: %q", out.Surface)
        }
        if !strings.Contains(out.FinalPrompt, "[User]:hi") {
                t.Fatalf("unexpected final prompt: %q", out.FinalPrompt)
        }
}
