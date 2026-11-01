package claude

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestReasoningToolProtocolNeverReachesVisibleDeltas(t *testing.T) {
	for _, raw := range []string{
		`plan <|Q7ZK3M|invoke name="run"><|Q7ZK3M|parameter name="cmd">secret</|Q7ZK3M|parameter></|Q7ZK3M|invoke> done`,
		`plan <｜epse｜call name="run">secret</｜epse｜call>`,
		`plan <|Q7ZK3M|invoke name="run" secret`,
		`plan ＜｜DSML｜ calls>secret`,
	} {
		rec := httptest.NewRecorder()
		rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, true, false, true, nil, nil, "prompt", nil)
		rt.sieve.Marker = "Q7ZK3M"
		for _, ch := range raw {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "thinking", Text: string(ch)}}})
		}
		rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "answer"}}})
		if !rt.finalize("end_turn", false) {
			t.Fatal("ordinary answer was deferred")
		}
		body := rec.Body.String()
		var reasoning strings.Builder
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]any
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) != nil {
				continue
			}
			collectVisibleReasoning(frame, &reasoning)
		}
		for _, forbidden := range []string{"secret", "EPSE", "epse", "Q7ZK3M", "DSML"} {
			if strings.Contains(reasoning.String(), forbidden) {
				t.Fatalf("reasoning protocol leaked: raw=%q body=%s", raw, body)
			}
		}
		if !strings.Contains(reasoning.String(), "plan ") || !strings.Contains(body, "answer") {
			t.Fatalf("ordinary content lost: %s", body)
		}
	}
}

func collectVisibleReasoning(value any, out *strings.Builder) {
	switch node := value.(type) {
	case map[string]any:
		for _, key := range []string{"reasoning_content", "thinking"} {
			if text, ok := node[key].(string); ok {
				out.WriteString(text)
			}
		}
		if node["thought"] == true {
			if text, ok := node["text"].(string); ok {
				out.WriteString(text)
			}
		}
		if node["type"] == "response.reasoning_summary_text.delta" {
			if text, ok := node["delta"].(string); ok {
				out.WriteString(text)
			}
		}
		for _, child := range node {
			collectVisibleReasoning(child, out)
		}
	case []any:
		for _, child := range node {
			collectVisibleReasoning(child, out)
		}
	}
}

func TestFilteredStreamCannotReleaseHeldToolText(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, true, false, true, nil, nil, "prompt", nil)
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: `answer <|EPSE|tool_calls>secret`}}})
	rt.onParsed(sse.LineResult{Parsed: true, ContentFilter: true})
	rt.finalize("end_turn", false)
	body := rec.Body.String()
	if strings.Contains(body, "secret") || strings.Contains(body, "EPSE") {
		t.Fatalf("filter released raw tool text: %s", body)
	}
}
