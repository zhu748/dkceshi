package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
)

type agentLoopDS struct {
	responsesHistoryDS
	answer string
}

func (d *agentLoopDS) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	d.payload = payload
	data, _ := json.Marshal(map[string]any{"p": "response/content", "v": d.answer})
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: " + string(data) + "\ndata: [DONE]\n"))}, nil
}
func TestAgentLoopContinuesTwelveToolRoundsByResponseID(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	ds := &agentLoopDS{}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	tools := []any{map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"step": map[string]any{"type": "integer"}}, "required": []any{"step"}}}}
	body := map[string]any{"model": "deepseek-v4.1-flash", "input": "original-agent-task", "tools": tools}
	send := func() map[string]any {
		data, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer direct-token")
		rec := httptest.NewRecorder()
		h.Responses(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%d: %s", rec.Code, rec.Body.String())
		}
		return decodeJSONBody(t, rec.Body.String())
	}
	seen := map[string]bool{}
	for step := 0; step < 12; step++ {
		ds.answer = fmt.Sprintf(`<tool_calls><invoke name="lookup"><parameter name="step">%d</parameter></invoke></tool_calls>`, step)
		response := send()
		output := response["output"].([]any)
		call := output[0].(map[string]any)
		id := call["call_id"].(string)
		if seen[id] {
			t.Fatalf("call identity reused: %s", id)
		}
		seen[id] = true
		body = map[string]any{"model": "deepseek-v4.1-flash", "tools": tools, "previous_response_id": response["id"], "input": []any{map[string]any{"type": "function_call_output", "call_id": id, "output": fmt.Sprintf("result-step-%d", step)}}}
	}
	ds.answer = "finished"
	last := send()
	if last["output_text"] != "finished" {
		t.Fatalf("unexpected final response: %#v", last)
	}
	prompt := ds.payload["prompt"].(string)
	for _, text := range []string{"original-agent-task", "result-step-0", "result-step-11"} {
		if !strings.Contains(prompt, text) {
			t.Fatalf("agent context lost %s", text)
		}
	}
	for id := range seen {
		if strings.Count(prompt, id) != 2 {
			t.Fatalf("call/result pair changed for %s", id)
		}
	}
}
