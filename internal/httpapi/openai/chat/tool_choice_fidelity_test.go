package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestConstrainedStreamDoesNotEmitForbiddenToolBeforeValidation(t *testing.T) {
	for _, policy := range []promptcompat.ToolChoicePolicy{
		{Mode: promptcompat.ToolChoiceNone},
		{Mode: promptcompat.ToolChoiceForced, ForcedName: "allowed"},
		{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"allowed": {}}},
	} {
		rec := httptest.NewRecorder()
		runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, true, []string{"forbidden"}, nil, policy, true, true)
		markup := `<|EPSE|tool_calls><|EPSE|invoke name="forbidden"><|EPSE|parameter name="x"><![CDATA[value]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
		// Split inside tags to exercise early tool deltas, not just final parsing.
		lines := []string{}
		for _, part := range []string{markup[:35], markup[35:90], markup[90:]} {
			payload, _ := json.Marshal(map[string]any{"p": "response/content", "v": part})
			lines = append(lines, "data: "+string(payload))
		}
		lines = append(lines, "data: [DONE]")
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		(&Handler{}).consumeChatStreamAttempt(req, makeOpenAISSEHTTPResponse(lines...), runtime, "text", false, nil, false)
		body := rec.Body.String()
		if !strings.Contains(body, "tool_choice_violation") || strings.Contains(body, `"tool_calls":[`) {
			t.Fatalf("forbidden tool escaped validation: %s", body)
		}
	}
}

func TestSequentialToolPolicyRejectsMultipleCallsBeforeAnyEmission(t *testing.T) {
	rec := httptest.NewRecorder()
	policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, DisableParallel: true}
	runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, true, []string{"lookup"}, nil, policy, true, true)
	lines := []string{}
	for _, value := range []string{"a", "b"} {
		payload, _ := json.Marshal(map[string]any{"p": "response/content", "v": `<tool_calls><invoke name="lookup"><parameter name="key">` + value + `</parameter></invoke></tool_calls>`})
		lines = append(lines, "data: "+string(payload))
	}
	lines = append(lines, "data: [DONE]")
	(&Handler{}).consumeChatStreamAttempt(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), makeOpenAISSEHTTPResponse(lines...), runtime, "text", false, nil, false)
	body := rec.Body.String()
	if !strings.Contains(body, "tool_choice_violation") || strings.Contains(body, `"tool_calls":[`) {
		t.Fatalf("parallel calls escaped sequential policy: %s", body)
	}
}

func TestRequiredToolChoiceDefersEmptyResponseForRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired}
	runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "deepseek-v4.1-flash", "prompt", false, false, true, []string{"lookup"}, nil, policy, true, true)
	if runtime.finalize("stop", true) || runtime.finalErrorCode != "upstream_unavailable" || rec.Body.Len() != 0 {
		t.Fatalf("empty response terminated before retry: code=%s body=%s", runtime.finalErrorCode, rec.Body.String())
	}
}
