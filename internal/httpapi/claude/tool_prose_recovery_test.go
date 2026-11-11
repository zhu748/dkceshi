package claude

import (
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolFailureAfterProseCanRecover(t *testing.T) {
	for _, textOnly := range []bool{false, true} {
		rec := httptest.NewRecorder()
		rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
		feed := func(text string) {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: text}}})
		}
		feed("Original preamble. <tool_calls><invoke name=\"read\"><parameter name=\"path\">")
		if rt.finalize("end_turn", true) || rt.finalErrorCode != "invalid_tool_arguments" {
			t.Fatalf("failure was not deferred: %s", rec.Body.String())
		}
		rt.resetForRegeneration()
		feed("DUPLICATE <tool_calls><invoke name=\"read\"><parameter name=\"path\">")
		if rt.finalize("end_turn", true) {
			t.Fatalf("second failure was not deferred: %s", rec.Body.String())
		}
		rt.resetForRegeneration()
		good := "DUPLICATE <tool_calls><invoke name=\"read\"><parameter name=\"path\">a.txt</parameter></invoke></tool_calls> FAKE_RESULT"
		if textOnly {
			good = "Recovered final answer."
		}
		feed(good)
		if !rt.finalize("end_turn", false) {
			t.Fatal("valid recovery did not finish")
		}
		body := rec.Body.String()
		if strings.Contains(body, "DUPLICATE") || strings.Contains(body, "invalid_tool_arguments") || strings.Contains(body, "FAKE_RESULT") || !strings.Contains(body, "Original preamble.") {
			t.Fatalf("invalid recovery delivery: %s", body)
		}
		if strings.Contains(body, "tool_use") == textOnly || textOnly && !strings.Contains(body, "Recovered final answer.") {
			t.Fatalf("missing recovery: %s", body)
		}
	}
}
