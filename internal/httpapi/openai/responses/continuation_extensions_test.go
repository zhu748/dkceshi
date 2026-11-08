package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContinuationRetainsNestedToolAndEmptyReasoningContext(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	ds := &responsesHistoryDS{}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	send := func(body map[string]any) map[string]any {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer direct-token")
		rec := httptest.NewRecorder()
		h.Responses(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		return decodeJSONBody(t, rec.Body.String())
	}
	first := send(map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
		map[string]any{"role": "user", "content": "ORIGINAL_TASK"},
		map[string]any{"type": "reasoning", "id": "r", "summary": []any{map[string]any{"type": "summary_text", "text": "", "annotations": []any{"EMPTY_REASONING_CONTEXT"}}}},
		map[string]any{"type": "function_call", "call_id": "call_a", "metadata": "ITEM_CONTEXT", "function": map[string]any{"name": "lookup", "arguments": "{}", "metadata": "FUNCTION_CONTEXT"}},
	}})
	id := first["id"].(string)
	send(map[string]any{"model": "deepseek-v4.1-flash", "previous_response_id": id, "input": []any{
		map[string]any{"type": "function_call_output", "call_id": "call_a", "output": "ACTUAL_RESULT", "metadata": "RESULT_CONTEXT"},
	}})
	prompt := ds.payload["prompt"].(string)
	for _, want := range []string{"ORIGINAL_TASK", "EMPTY_REASONING_CONTEXT", "ITEM_CONTEXT", "FUNCTION_CONTEXT", "ACTUAL_RESULT", "RESULT_CONTEXT", `call_id="call_a"`, `"tool_call_id":"call_a"`} {
		if strings.Count(prompt, want) != 1 {
			t.Fatalf("continuation lost or duplicated %q: %s", want, prompt)
		}
	}
}
