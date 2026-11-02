package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/completionruntime"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestCatalogProseAndThinkingDelivery(t *testing.T) {
	for _, mode := range []promptcompat.ToolChoiceMode{promptcompat.ToolChoiceAuto, promptcompat.ToolChoiceNone, promptcompat.ToolChoiceRequired} {
		rec := httptest.NewRecorder()
		rt := newGeminiStreamRuntime(rec, http.NewResponseController(rec), true, "model", "prompt", true, false, true, []string{"read"}, nil, true, nil)
		rt.toolChoice.Mode = mode
		rt.bufferContent = completionruntime.HoldToolOutput(rt.toolNames, rt.toolsRaw, rt.toolChoice)
		for _, part := range []sse.ContentPart{{Type: "thinking", Text: "Checking details."}, {Type: "text", Text: "\n"}, {Type: "text", Text: "First part."}, {Type: "text", Text: " Second part."}} {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{part}})
		}
		before := rec.Body.String()
		if !strings.Contains(before, `"thought":true`) || strings.Contains(before, "First part.") == rt.toolChoice.IsRequired() {
			t.Fatalf("mode=%s wrong streaming boundary: %s", mode, before)
		}
		if rt.toolChoice.IsRequired() {
			feedGeminiText(rt, `<tool_calls><invoke name="read"></invoke></tool_calls> FAKE_RESULT`)
			if strings.Contains(rec.Body.String(), "functionCall") {
				t.Fatal("unvalidated tool delivered")
			}
		}
		if !rt.finalize(false) || strings.Contains(rec.Body.String(), `"error"`) || strings.Count(rec.Body.String(), "Checking details.") != 1 || strings.Contains(rec.Body.String(), "FAKE_RESULT") {
			t.Fatalf("mode=%s duplicate/invalid output: %s", mode, rec.Body.String())
		}
		if strings.Count(rec.Body.String(), "First part.") != 1 {
			t.Fatal("answer duplicated or lost")
		}
	}
}
