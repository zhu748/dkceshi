package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestStreamRejectsUnrepairableClientCallWithoutLeakingMarkupOrPartialCalls(t *testing.T) {
	good := `<|EPSE|tool_calls><|EPSE|invoke name="read"></|EPSE|invoke></|EPSE|tool_calls>`
	bad := `<|EPSE|call name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter></|EPSE|call>`
	for _, input := range []string{bad, good + bad, `<｜DSML｜calls><｜DSML｜invoke name="write"><｜DSML｜parameter name="path" string="true">a.txt`} {
		for _, size := range []int{1, 7, len(input)} {
			rec := httptest.NewRecorder()
			policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"read": {}, "write": {}}}
			rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"read", "write"}, nil, policy, true, true)
			for i := 0; i < len(input); i += size {
				rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: input[i:min(i+size, len(input))]}}})
			}
			rt.finalize("stop", false)
			frames, _ := parseSSEDataFrames(t, rec.Body.String())
			if rt.finalErrorCode != "invalid_tool_arguments" || streamHasToolCallsDelta(frames) || collectStreamContent(frames) != "" {
				t.Fatalf("size=%d error=%s body=%s", size, rt.finalErrorCode, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "EPSE") || strings.Contains(rec.Body.String(), "DSML") {
				t.Fatalf("protocol markup leaked in error response: %s", rec.Body.String())
			}
		}
	}
}

func TestSchemaLessStreamWaitsForTheWholeBatchBeforeDelivery(t *testing.T) {
	good := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	bad := `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`
	rec := httptest.NewRecorder()
	rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, nil, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
	for _, text := range []string{good, bad} {
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: text}}})
		if rec.Body.Len() != 0 {
			t.Fatalf("partial batch delivered: %s", rec.Body.String())
		}
	}
	if rt.finalize("stop", true) || rt.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("broken schema-less batch was accepted: code=%s body=%s", rt.finalErrorCode, rec.Body.String())
	}
	rt.resetForRegeneration()
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "corrected answer"}}})
	rt.finalize("stop", false)
	frames, _ := parseSSEDataFrames(t, rec.Body.String())
	if streamHasToolCallsDelta(frames) || collectStreamContent(frames) != "corrected answer" {
		t.Fatalf("retry polluted by old batch: %s", rec.Body.String())
	}
}
