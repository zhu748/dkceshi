package chat

import (
	"context"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegressionRepairedMarkupDoesNotLeakAsContent(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, true, []string{"Bash"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
	rt.toolCallRepair = func(context.Context, string) (string, error) {
		return `<|EPSE|tool_calls><|EPSE|invoke name="Bash"><|EPSE|parameter name="command"><![CDATA[DS_SLOT_0]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`, nil
	}
	bad := `<|EPSE|call name="Bash"><|EPSE|parameter name="command"><![CDATA[pwd]]></|EPSE|parameter></|EPSE|call>`
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: bad}}})
	rt.finalize("stop", false)
	body := rec.Body.String()
	if !strings.Contains(body, `"tool_calls":[`) {
		t.Fatalf("repair did not generate tool call: %s", body)
	}
	if strings.Contains(body, `"content":`) {
		t.Fatalf("bad markup already escaped as visible content before successful repair: %s", body)
	}
}

func TestMalformedPrefixBeforeValidCallDoesNotLeak(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"Bash"}, nil, promptcompat.DefaultToolChoicePolicy(), true, true)
	bad := `<|EPSE|call name="Bash"><|EPSE|parameter name="command"><![CDATA[bad]]></|EPSE|parameter></|EPSE|call>`
	good := `<|EPSE|tool_calls><|EPSE|invoke name="Bash"><|EPSE|parameter name="command"><![CDATA[pwd]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: bad + good}}})
	rt.finalize("stop", false)
	if strings.Contains(rec.Body.String(), `"content":`) {
		t.Fatalf("malformed prefix leaked: %s", rec.Body.String())
	}
}
