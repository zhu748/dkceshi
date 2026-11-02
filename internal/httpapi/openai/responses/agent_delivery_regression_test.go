package responses

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegressionToolStreamUsage(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", false, false, false, []string{"write"}, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + strings.Repeat("hello world ", 100) + `]]></parameter></invoke></tool_calls>`
	s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw}}})
	s.finalize("stop", false)
	payload, ok := extractSSEEventPayload(rec.Body.String(), "response.completed")
	if !ok {
		t.Fatal(rec.Body.String())
	}
	usage := payload["response"].(map[string]any)["usage"].(map[string]any)
	t.Logf("usage=%v", usage)
	if usage["output_tokens"] == float64(0) {
		t.Fatal("1200 bytes of tool arguments counted as zero output tokens")
	}
}
func TestRegressionReasoningAfterWhitespaceRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", true, false, false, nil, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	feed := func(typ, txt string) {
		s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: typ, Text: txt}}})
	}
	feed("thinking", "first thought")
	feed("text", "\n\n")
	terminal := s.finalize("stop", true)
	t.Logf("terminal=%v reasoningClosed=%v error=%s", terminal, s.reasoningClosed, s.finalErrorCode)
	if terminal {
		t.Fatal("expected retry")
	}
	s.resetForRegeneration()
	offset := rec.Body.Len()
	feed("thinking", "second thought")
	if s.reasoningClosed || !strings.Contains(rec.Body.String()[offset:], "event: response.reasoning_text.delta") {
		t.Fatalf("reasoning item was closed before retry: %s", rec.Body.String())
	}
	feed("text", "\nCorrected answer")
	s.finalize("stop", false)
	body := rec.Body.String()
	if strings.Count(body, "event: response.reasoning_text.done") != 1 || strings.LastIndex(body, "event: response.reasoning_text.delta") > strings.Index(body, "event: response.reasoning_text.done") {
		t.Fatalf("invalid reasoning event lifecycle: %s", body)
	}
	completed, ok := extractSSEEventPayload(body, "response.completed")
	if !ok || completed["response"].(map[string]any)["output_text"] != "\nCorrected answer" {
		t.Fatalf("failed whitespace retained or successful prefix lost: %s", body)
	}
}

func TestRecoveredDSMLCannotEmitTail(t *testing.T) {
	raw := `before <｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls> FAKE_RESULT`
	for _, names := range [][]string{nil, {"read"}} {
		for _, size := range []int{1, len(raw)} {
			rec := httptest.NewRecorder()
			s := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", false, false, false, names, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
			for i := 0; i < len(raw); i += size {
				s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
			}
			s.finalize("stop", false)
			if strings.Contains(rec.Body.String(), "FAKE_RESULT") || !strings.Contains(rec.Body.String(), `"type":"function_call"`) {
				t.Fatalf("names=%v size=%d: %s", names, size, rec.Body.String())
			}
		}
	}
}
