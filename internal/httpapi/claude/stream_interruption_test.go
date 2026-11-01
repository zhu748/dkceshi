package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failedClaudeReader struct{}

func (failedClaudeReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestClaudeContentFilterNeverEntersEmptyRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, nil, nil, "prompt", nil)
	response := makeClaudeSSEHTTPResponse(`data: {"p":"response/status","v":"CONTENT_FILTER"}`)
	result := (&Handler{}).consumeClaudeStreamAttempt(httptest.NewRequest("POST", "/", nil), response, runtime, false, true)
	if !result.Terminal || result.Retryable || !strings.Contains(rec.Body.String(), "content_filter") {
		t.Fatalf("content filter lost: %+v %s", result, rec.Body.String())
	}
}

func TestClaudeInterruptedAnswerResumesWithoutChangingChannels(t *testing.T) {
	for _, cleanEOF := range []bool{false, true} {
		rec := httptest.NewRecorder()
		runtime := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, true, false, true, nil, nil, "prompt", nil)
		payload := "data: {\"response_message_id\":77,\"p\":\"response/content\",\"v\":\"first part \"}\n"
		var body io.Reader = strings.NewReader(payload)
		if !cleanEOF {
			body = io.MultiReader(body, failedClaudeReader{})
		}
		req := httptest.NewRequest("POST", "/", nil)
		h := &Handler{}
		result := h.consumeClaudeStreamAttempt(req, &http.Response{StatusCode: 200, Body: io.NopCloser(body)}, runtime, true, true)
		if result.Terminal || !result.ResumeContinue || runtime.responseMessageID != 77 {
			t.Fatalf("resume not scheduled: %+v %s", result, rec.Body.String())
		}
		result = h.consumeClaudeStreamAttempt(req, makeClaudeSSEHTTPResponse(`data: {"v":"second part"}`, `data: [DONE]`), runtime, true, true)
		text := collectClaudeTextDeltas(parseClaudeFrames(t, rec.Body.String()))
		if !result.Terminal || text != "first part second part" || strings.Contains(rec.Body.String(), "event: error") {
			t.Fatalf("resume lost answer: %+v text=%q frames=%s", result, text, rec.Body.String())
		}
	}
}

func TestClaudeToolBatchCanCompleteAfterResume(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newClaudeStreamRuntime(rec, http.NewResponseController(rec), true, "model", nil, false, false, true, []string{"run"}, nil, "prompt", nil)
	runtime.responseMessageID = 77
	req := httptest.NewRequest("POST", "/", nil)
	first := `<tool_calls><invoke name="run"><parameter name="command"><![CDATA[echo ok`
	last := `]]></parameter></invoke></tool_calls>`
	result := (&Handler{}).consumeClaudeStreamAttempt(req, makeClaudeSSEHTTPResponse(makeClaudeContentLine(t, first)), runtime, false, true)
	if !result.ResumeContinue || strings.Contains(rec.Body.String(), "tool_use") {
		t.Fatalf("partial tool batch executed: %+v %s", result, rec.Body.String())
	}
	result = (&Handler{}).consumeClaudeStreamAttempt(req, makeClaudeSSEHTTPResponse(makeClaudeContentLine(t, last), "data: [DONE]"), runtime, false, true)
	if !result.Terminal || strings.Count(rec.Body.String(), `"type":"tool_use"`) != 1 || strings.Contains(rec.Body.String(), "event: error") {
		t.Fatalf("resumed tool batch failed: %+v %s", result, rec.Body.String())
	}
}
