package chat

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegressionDriftedMarkerMustNotLeakBeforeFinalToolCall(t *testing.T) {
	for _, marker := range []string{"Q7ZK3N", "EPS"} {
		raw := `<|` + marker + `|tool_calls><|` + marker + `|invoke name="read"><|` + marker + `|parameter name="path"><![CDATA[a.txt]]></|` + marker + `|parameter></|` + marker + `|invoke></|` + marker + `|tool_calls>`
		rec := httptest.NewRecorder()
		policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"read": {}}}
		rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"read"}, []any{}, policy, true, true)
		rt.toolSieve.Marker = "Q7ZK3M"
		for i := 0; i < len(raw); i++ {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i : i+1]}}})
		}
		rt.finalize("stop", false)
		frames, _ := parseSSEDataFrames(t, rec.Body.String())
		if content := collectStreamContent(frames); content != "" {
			t.Errorf("marker=%s structuredCalls=%v leaked=%q", marker, streamHasToolCallsDelta(frames), content)
		}
	}
}
