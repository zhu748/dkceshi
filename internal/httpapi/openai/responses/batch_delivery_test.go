package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestSchemaLessStreamCannotDeliverAPartialBatch(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "model", "prompt", false, false, true, nil, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	for _, text := range []string{`<tool_calls><invoke name="read"></invoke></tool_calls>`, `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`} {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: text}}})
		if rec.Body.Len() != 0 {
			t.Fatalf("partial response item delivered: %s", rec.Body.String())
		}
	}
	if rt.finalize("stop", true) || rt.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("invalid batch was accepted: %s", rec.Body.String())
	}
	rt.resetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "corrected answer"}}})
	rt.finalize("stop", false)
	if !strings.Contains(rec.Body.String(), "response.completed") || strings.Contains(rec.Body.String(), "function_call") {
		t.Fatalf("retry polluted: %s", rec.Body.String())
	}
}
