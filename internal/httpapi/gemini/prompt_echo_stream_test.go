package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestGeminiPromptEchoRejectsBeforeContentDelivery(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	rec := httptest.NewRecorder()
	rt := newGeminiStreamRuntime(rec, http.NewResponseController(rec), true, "model", prefix, true, false, true, nil, nil, false, nil)
	for _, ch := range prefix {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: string(ch)}}})
	}
	if rt.finalize(true) || rt.finalErrorCode != "upstream_prompt_echo" || strings.Contains(rec.Body.String(), "candidates") {
		t.Fatalf("echo escaped: code=%s body=%s", rt.finalErrorCode, rec.Body.String())
	}
	rt.accumulator.ResetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "healthy answer"}}})
	if !rt.finalize(true) || !strings.Contains(rec.Body.String(), "healthy answer") {
		t.Fatalf("clean retry failed: %s", rec.Body.String())
	}
}
