package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestInvalidAttributeBatchNeverLeaksOrDeliversToolsAtAnySplit(t *testing.T) {
	raw := `<tool_calls><invoke name="read"><parameter name="path" value="TARGET.txt"></parameter></invoke></tool_calls>`
	for split := 0; split <= len(raw); split++ {
		rec := httptest.NewRecorder()
		s := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, false, []string{"read"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
		for _, part := range []string{raw[:split], raw[split:]} {
			s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: part}}})
		}
		s.finalize("stop", false)
		frames, _ := parseSSEDataFrames(t, rec.Body.String())
		if content := collectStreamContent(frames); content != "" || strings.Contains(rec.Body.String(), `"tool_calls":[`) || s.finalErrorCode != "invalid_tool_arguments" {
			t.Fatalf("split=%d invalid attribute batch escaped: %s", split, rec.Body.String())
		}
	}
}
