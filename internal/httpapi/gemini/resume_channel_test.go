package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiResumeKeepsPathlessTextOnBodyChannel(t *testing.T) {
	rec := httptest.NewRecorder()
	rt := newGeminiStreamRuntime(rec, http.NewResponseController(rec), true, "model", "prompt", true, false, true, nil, nil, false, nil)
	response := func(wire string) *http.Response {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire))}
	}
	h := &Handler{}
	first := response("data: {\"response_message_id\":92,\"p\":\"response/content\",\"v\":\"first \"}\n\n")
	if result := h.consumeGeminiStreamAttempt(context.Background(), first, rt, true, true); !result.ResumeContinue {
		t.Fatalf("expected resume: %+v", result)
	}
	second := response("data: {\"v\":\"second\"}\n\ndata: [DONE]\n\n")
	result := h.consumeGeminiStreamAttempt(context.Background(), second, rt, true, false)
	raw, _, _, _, _ := rt.accumulator.Snapshot()
	if !result.Terminal || raw != "first second" || rt.finalErrorCode != "" {
		t.Fatalf("body moved into reasoning: result=%+v raw=%q body=%s", result, raw, rec.Body.String())
	}
}
