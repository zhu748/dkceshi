package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestSeparateToolBlocksKeepIdentityThroughCompletedResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", false, false, true, []string{"lookup"}, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	lines := []string{}
	for _, value := range []string{"a", "b"} {
		payload, _ := json.Marshal(map[string]any{"p": "response/content", "v": `<tool_calls><invoke name="lookup"><parameter name="value">` + value + `</parameter></invoke></tool_calls>`})
		lines = append(lines, "data: "+string(payload))
	}
	lines = append(lines, "data: [DONE]")
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	(&Handler{}).consumeResponsesStreamAttempt(req, makeResponsesOpenAISSEHTTPResponse(lines...), runtime, "text", false, false)
	var done []any
	var completed []any
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["type"] == "response.output_item.done" {
			if item, ok := payload["item"].(map[string]any); ok && item["type"] == "function_call" {
				done = append(done, item)
			}
		}
		if payload["type"] == "response.completed" {
			response, _ := payload["response"].(map[string]any)
			completed, _ = response["output"].([]any)
		}
	}
	if len(done) != 2 || !reflect.DeepEqual(done, completed) {
		t.Fatalf("streamed calls changed in completed response: done=%#v completed=%#v", done, completed)
	}
}

func TestConstrainedStreamValidatesBeforeEmittingFunctionItems(t *testing.T) {
	for _, tc := range []struct {
		policy promptcompat.ToolChoicePolicy
		valid  bool
	}{
		{promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceNone}, false},
		{promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceForced, ForcedName: "other"}, false},
		{promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired}, true},
		{promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceForced, ForcedName: "lookup"}, true},
	} {
		rec := httptest.NewRecorder()
		runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", false, false, true, []string{"lookup"}, nil, true, true, tc.policy, "", nil, nil)
		markup := `<|EPSE|tool_calls><|EPSE|invoke name="lookup"><|EPSE|parameter name="x"><![CDATA[value]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
		lines := []string{}
		for _, part := range []string{markup[:35], markup[35:90], markup[90:]} {
			payload, _ := json.Marshal(map[string]any{"p": "response/content", "v": part})
			lines = append(lines, "data: "+string(payload))
		}
		lines = append(lines, "data: [DONE]")
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		(&Handler{}).consumeResponsesStreamAttempt(req, makeResponsesOpenAISSEHTTPResponse(lines...), runtime, "text", false, false)
		body := rec.Body.String()
		if tc.valid {
			if !strings.Contains(body, "response.completed") || !strings.Contains(body, `"type":"function_call"`) || strings.Contains(body, "response.failed") {
				t.Fatalf("valid call was not delivered: %s", body)
			}
		} else if !strings.Contains(body, "tool_choice_violation") || strings.Contains(body, `"type":"function_call"`) || strings.Contains(body, "response.completed") {
			t.Fatalf("forbidden call escaped validation: %s", body)
		}
	}
}

func TestRequiredToolChoiceDefersEmptyResponseForRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceRequired}
	runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "id", "deepseek-v4.1-flash", "prompt", false, false, true, []string{"lookup"}, nil, true, true, policy, "", nil, nil)
	if runtime.finalize("stop", true) || runtime.finalErrorCode != "upstream_unavailable" || strings.Contains(rec.Body.String(), "response.failed") {
		t.Fatalf("empty response terminated before retry: code=%s body=%s", runtime.finalErrorCode, rec.Body.String())
	}
}
