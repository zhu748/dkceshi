package claude

import (
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegressionHiddenReasoningMustCountAsUpstreamActivity(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
	result := rt.onParsed(sse.LineResult{Parsed: true, ToolDetectionThinkingParts: []sse.ContentPart{{Type: "thinking", Text: "actively planning a tool call"}}})
	if !result.ContentSeen {
		t.Fatalf("active hidden reasoning did not reset watchdog: buffered=%q", rt.toolDetectionThinking.String())
	}
}

func TestRawToolMarkupResetsActivityBeforeVisibleCleanup(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, []string{"read"}, []any{}, "prompt", nil)
	result := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: `<tool_calls><invoke name="read"></invoke></tool_calls>`}}})
	if !result.ContentSeen {
		t.Fatal("raw tool activity was lost during output cleanup")
	}
}
