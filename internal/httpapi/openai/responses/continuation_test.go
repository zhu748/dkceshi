package responses

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegressionUnknownPreviousResponseRejected(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	ds := &responsesHistoryDS{}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-v4.1-flash","previous_response_id":"resp_missing","input":[{"type":"function_call_output","call_id":"call_old","output":"done"}]}`))
	req.Header.Set("Authorization", "Bearer direct-token")
	rec := httptest.NewRecorder()
	h.Responses(rec, req)
	if rec.Code < 400 {
		t.Fatalf("unknown previous_response_id silently accepted: status=%d upstream_called=%v", rec.Code, ds.payload != nil)
	}
}

func TestContinuationReplaysCallsAndResultsWithoutOldInstructions(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	ds := &responsesHistoryDS{}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	send := func(body, token string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.Responses(rec, req)
		return rec, decodeJSONBody(t, rec.Body.String())
	}
	first, obj := send(`{"model":"deepseek-v4.1-flash","instructions":"only-first-instructions","input":"unique-original-task"}`, "direct-token")
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	id := obj["id"].(string)
	// Record an assistant tool call as though it was returned in the first response.
	owner := responseStoreOwner(authForToken(t, resolver, "direct-token"))
	context, _ := h.getResponseStore().context(owner, id)
	context = append(context, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call_a", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}})
	h.getResponseStore().putContext(owner, id, obj, context)
	second, obj2 := send(`{"model":"deepseek-v4.1-flash","instructions":"second-instructions","previous_response_id":"`+id+`","input":[{"type":"function_call_output","call_id":"call_a","output":"actual-tool-result"}]}`, "direct-token")
	if second.Code != 200 {
		t.Fatal(second.Body.String())
	}
	prompt := ds.payload["prompt"].(string)
	for _, part := range []string{"unique-original-task", "second-instructions", "call_a", "lookup", "actual-tool-result"} {
		if !strings.Contains(prompt, part) {
			t.Fatalf("missing %s: %s", part, prompt)
		}
	}
	if strings.Contains(prompt, "only-first-instructions") {
		t.Fatal("top-level instructions leaked into next response")
	}
	if obj2["previous_response_id"] != id {
		t.Fatal("lineage missing")
	}
	cross, _ := send(`{"model":"deepseek-v4.1-flash","previous_response_id":"`+id+`","input":"next"}`, "other-token")
	if cross.Code < 400 {
		t.Fatal("cross-caller continuation accepted")
	}
	noStore, obj3 := send(`{"model":"deepseek-v4.1-flash","store":false,"input":"private"}`, "direct-token")
	if noStore.Code != 200 {
		t.Fatal(noStore.Body.String())
	}
	if _, ok := h.getResponseStore().get(owner, obj3["id"].(string)); ok {
		t.Fatal("store:false response was retained")
	}
}

func TestResponseStoreClonesNestedContext(t *testing.T) {
	s := newResponseStore(0)
	value := map[string]any{"output": []any{map[string]any{"text": "original"}}}
	context := []any{map[string]any{"role": "user", "content": "original"}}
	s.putContext("owner", "resp", value, context)
	value["output"].([]any)[0].(map[string]any)["text"] = "mutated"
	context[0].(map[string]any)["content"] = "mutated"
	got, _ := s.get("owner", "resp")
	got["output"].([]any)[0].(map[string]any)["text"] = "mutated again"
	fresh, _ := s.get("owner", "resp")
	replay, _ := s.context("owner", "resp")
	if fresh["output"].([]any)[0].(map[string]any)["text"] != "original" || replay[0].(map[string]any)["content"] != "original" {
		t.Fatal("store aliases mutable nested data")
	}
}
