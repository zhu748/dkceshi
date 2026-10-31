package claude

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/sse"
)

func TestSchemaLessStreamCannotDeliverAPartialBatch(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
	rt.bufferToolContent = true
	for _, text := range []string{`<tool_calls><invoke name="read"></invoke></tool_calls>`, `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`} {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: text}}})
		if rec.Body.Len() != 0 {
			t.Fatalf("partial tool use delivered: %s", rec.Body.String())
		}
	}
	if rt.finalize("end_turn", true) || rt.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("invalid batch was accepted: %s", rec.Body.String())
	}
	rt.resetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "corrected answer"}}})
	rt.finalize("end_turn", false)
	if got := collectClaudeTextDeltas(parseClaudeFrames(t, rec.Body.String())); got != "corrected answer" {
		t.Fatalf("retry polluted: %q", got)
	}
}
