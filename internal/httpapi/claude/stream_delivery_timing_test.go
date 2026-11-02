package claude

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
		rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, true, false, true, []string{"read"}, nil, "prompt", nil)
		rt.toolChoice.Mode = mode
		for _, part := range []sse.ContentPart{{Type: "thinking", Text: "Checking details."}, {Type: "text", Text: "\n"}, {Type: "text", Text: "First part."}, {Type: "text", Text: " Second part."}} {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{part}})
		}
		if got := collectClaudeTextDeltas(parseClaudeFrames(t, rec.Body.String())); got != "\nFirst part. Second part." || !strings.Contains(rec.Body.String(), "thinking_delta") {
			t.Fatalf("mode=%s delayed answer: %s", mode, rec.Body.String())
		}
		rt.finalize("end_turn", false)
		if got := collectClaudeTextDeltas(parseClaudeFrames(t, rec.Body.String())); got != "\nFirst part. Second part." || strings.Contains(rec.Body.String(), "event: error") {
			t.Fatalf("answer duplicated/rejected: %s", rec.Body.String())
		}
	}
}
