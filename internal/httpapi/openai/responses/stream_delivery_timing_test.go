package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestCatalogAnswerStreamsBeforeFinalization(t *testing.T) {
	for _, mode := range []promptcompat.ToolChoiceMode{promptcompat.ToolChoiceAuto, promptcompat.ToolChoiceNone} {
		rec := httptest.NewRecorder()
		rt := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "model", "prompt", true, false, true, []string{"read"}, nil, true, true, promptcompat.ToolChoicePolicy{Mode: mode}, "", nil, nil)
		for _, part := range []sse.ContentPart{{Type: "thinking", Text: "Checking details."}, {Type: "text", Text: "\n"}, {Type: "text", Text: "First part."}, {Type: "text", Text: " Second part."}} {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{part}})
		}
		before := rec.Body.String()
		if rt.visibleText.String() != "\nFirst part. Second part." || !strings.Contains(before, "response.reasoning_text.delta") || strings.Contains(before, "response.completed") {
			t.Fatalf("mode=%s delayed answer: %s", mode, before)
		}
		if !rt.finalize("stop", false) || rt.visibleText.String() != "\nFirst part. Second part." || strings.Contains(rec.Body.String(), "response.failed") {
			t.Fatalf("answer was duplicated/rejected: %s", rec.Body.String())
		}
	}
}
