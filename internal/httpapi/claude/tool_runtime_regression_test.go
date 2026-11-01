package claude

import (
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolRuntimeRegressionToolThenUpstreamError(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "deepseek-v4.1-flash", nil, false, false, false, []string{"lookup"}, nil, "prompt", nil)
	resp := makeClaudeSSEHTTPResponse(makeClaudeContentLine(t, `<tool_calls><invoke name="lookup"></invoke></tool_calls>`), `data: {"error":{"message":"内容超长，请删减后再试"}}`)
	(&Handler{}).consumeClaudeStreamAttempt(httptest.NewRequest(http.MethodPost, "/", nil), resp, s, false, false)
	if strings.Contains(rec.Body.String(), `"type":"tool_use"`) {
		t.Errorf("explicit upstream error still emits executable tool_use: %s", rec.Body.String())
	}
}

func TestToolRuntimeRegressionMalformedSiblingUnderContentFilter(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "deepseek-v4.1-flash", nil, false, false, false, []string{"read", "write"}, nil, "prompt", nil)
	s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: `<tool_calls><invoke name="read"></invoke><invoke name="write"><param name="path">lost.txt</param></invoke></tool_calls>`}}})
	s.onParsed(sse.LineResult{Parsed: true, ContentFilter: true})
	s.finalize("end_turn", false)
	if strings.Contains(rec.Body.String(), `"type":"tool_use"`) {
		t.Errorf("filtered batch drops malformed sibling and emits partial tools: %s", rec.Body.String())
	}
}

func TestSearchCitationLikeChunkRemainsAToolArgument(t *testing.T) {
	rec := httptest.NewRecorder()
	s := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "deepseek-v4.1-flash", nil, false, true, true, []string{"write"}, nil, "prompt", nil)
	for _, part := range []string{`<tool_calls><invoke name="write"><parameter name="content"><![CDATA[`, `[citation:7] literal data`, `]]></parameter></invoke></tool_calls>`} {
		s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: part}}})
	}
	s.finalize("end_turn", false)
	if !strings.Contains(rec.Body.String(), `[citation:7] literal data`) {
		t.Fatalf("search display filtering mutated the delivered tool argument: %s", rec.Body.String())
	}
}
