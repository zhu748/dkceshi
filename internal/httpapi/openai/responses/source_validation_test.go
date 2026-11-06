package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidResponsesSourcesFailBeforeContinuationOrUpstream(t *testing.T) {
	// Nil collaborators ensure a malformed source cannot reach authentication,
	// continuation lookup, uploads or the upstream completion service.
	h := &Handler{}
	for _, body := range []string{
		`{"model":"deepseek-v4.1-flash","instructions":{"text":"LOST"},"input":"valid"}`,
		`{"model":"deepseek-v4.1-flash","input":[{"type":"reasoning","summary":[{"text":42}]},"valid"]}`,
		`{"model":"deepseek-v4.1-flash","input":[{"type":"function_call","name":"read","function":{"name":"different"},"arguments":"{}"},"valid"]}`,
		`{"model":"deepseek-v4.1-flash","previous_response_id":"unqueried","input":[{"type":"function_call_output","call_id":"one","tool_call_id":"other","output":"result"}]}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		recorder := httptest.NewRecorder()
		h.Responses(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid source status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}
