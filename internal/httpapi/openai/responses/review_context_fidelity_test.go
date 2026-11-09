package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContinuationPreservesResponsesItemAndCallIdentities(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	ds := &responsesHistoryDS{}
	handler := &Handler{Store: store, Auth: resolver, DS: ds}
	send := func(body map[string]any) map[string]any {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer direct-token")
		recorder := httptest.NewRecorder()
		handler.Responses(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatal(recorder.Body.String())
		}
		return decodeJSONBody(t, recorder.Body.String())
	}
	first := send(map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
		map[string]any{"role": "user", "content": "ORIGINAL_TASK"},
		map[string]any{"type": "function_call", "id": "fc-original", "call_id": "call-original", "name": "lookup", "arguments": `{"n":9007199254740993}`},
		map[string]any{"type": "function_call_output", "call_id": "call-original", "output": "ORIGINAL_RESULT"},
	}})
	send(map[string]any{"model": "deepseek-v4.1-flash", "previous_response_id": first["id"], "input": "LATEST_TASK"})
	prompt := ds.payload["prompt"].(string)
	for _, want := range []string{"ORIGINAL_TASK", "ORIGINAL_RESULT", "fc-original", "call-original", "9007199254740993", "LATEST_TASK"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("continuation lost %s", want)
		}
	}
}
