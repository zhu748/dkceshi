package claude

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestClaudePromptEchoCanRegenerateWithoutLeakingContent(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	rec := httptest.NewRecorder()
	rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, true, false, true, nil, nil, prefix, nil)
	for _, ch := range prefix {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: string(ch)}}})
	}
	if rt.finalize("stop", true) || rt.finalErrorCode != "upstream_prompt_echo" || strings.Contains(rec.Body.String(), "text_delta") || strings.Contains(rec.Body.String(), "message_stop") {
		t.Fatalf("echo escaped: code=%s body=%s", rt.finalErrorCode, rec.Body.String())
	}
	rt.resetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "healthy answer"}}})
	if !rt.finalize("stop", true) || !strings.Contains(rec.Body.String(), "healthy answer") {
		t.Fatalf("clean retry failed: %s", rec.Body.String())
	}
}
