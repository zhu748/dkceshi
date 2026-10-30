package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestStreamTranscriptBoundaryKeepsRealCallAndCutsInventedCall(t *testing.T) {
	const marker = "Q7ZK3M"
	call := `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="read"><|Q7ZK3M|parameter name="file_path"><![CDATA[a.txt]]></|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	input := call + "</|Q7ZK3M|tool_calls>\n[EndMessage:3237101fbf162c60]\n[Assistant]:" + strings.ReplaceAll(call, "a.txt", "invented.txt")
	for _, size := range []int{1, 2, 7, len(input)} {
		rec := httptest.NewRecorder()
		rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"read"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
		rt.toolSieve.Marker = marker
		stopped := false
		for i := 0; i < len(input); i += size {
			decision := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: input[i:min(i+size, len(input))]}}})
			if decision.Stop {
				stopped = true
				break
			}
		}
		rt.finalize("stop", false)
		body := rec.Body.String()
		if !stopped || rt.finalFinishReason != "tool_calls" || strings.Count(body, `"function":`) != 1 || !strings.Contains(body, "a.txt") {
			t.Fatalf("size=%d stopped=%v finish=%s body=%s", size, stopped, rt.finalFinishReason, body)
		}
		for _, bad := range []string{"EndMessage", "invented.txt", `"content":`, "Q7ZK3M"} {
			if strings.Contains(body, bad) {
				t.Fatalf("size=%d leaked %q body=%s", size, bad, body)
			}
		}
	}
}
