package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestResponsesPromptEchoCanRegenerateWithoutLeakingContent(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	rec := httptest.NewRecorder()
	rt := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "model", prefix, true, false, true, nil, nil, false, false, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	for _, ch := range prefix {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: string(ch)}}})
	}
	if rt.finalize("stop", true) || rt.finalErrorCode != "upstream_prompt_echo" || strings.Contains(rec.Body.String(), "output_text.delta") || strings.Contains(rec.Body.String(), "response.completed") {
		t.Fatalf("echo escaped: code=%s body=%s", rt.finalErrorCode, rec.Body.String())
	}
	rt.resetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "healthy answer"}}})
	if !rt.finalize("stop", true) || !strings.Contains(rec.Body.String(), "response.completed") {
		t.Fatalf("clean retry failed: %s", rec.Body.String())
	}
}
