package responses

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPersistenceCapacityFailureIsReported(t *testing.T) {
	h := &Handler{responses: newResponseStore(time.Hour)}
	h.responses.limits.bytes = 1 << 20
	value := map[string]any{"id": "resp_large", "output": []any{}}
	context := []any{map[string]any{"role": "user", "content": strings.Repeat("a", 2<<20)}}
	if h.responsePersister("owner", "resp_large", map[string]any{}, context)(value) || value["store"] != false {
		t.Fatal("failed write advertised persistence")
	}
	if _, ok := h.responses.context("owner", "resp_large"); ok {
		t.Fatal("oversized write retained")
	}
	value = map[string]any{"id": "resp_private", "output": []any{}}
	if !h.responsePersister("owner", "resp_private", map[string]any{"store": false}, context)(value) || value["store"] != false {
		t.Fatal("store:false failed due to capacity")
	}
}

func TestStreamResponsesPersistReplayContextAndHonorStoreFalse(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		store, resolver := newDirectTokenResolver(t)
		h := &Handler{Store: store, Auth: resolver, DS: &responsesHistoryDS{}}
		body := map[string]any{"model": "deepseek-v4.1-flash", "stream": true, "store": enabled, "input": "stream original task"}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer direct-token")
		rec := httptest.NewRecorder()
		h.Responses(rec, req)
		var response map[string]any
		for _, line := range strings.Split(rec.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data: {") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event["type"] == "response.completed" {
				response, _ = event["response"].(map[string]any)
			}
		}
		if response == nil {
			t.Fatalf("stream did not complete: %s", rec.Body.String())
		}
		if response["store"] != enabled {
			t.Fatalf("store flag lost: %+v", response)
		}
		owner := responseStoreOwner(authForToken(t, resolver, "direct-token"))
		id := response["id"].(string)
		context, found := h.getResponseStore().context(owner, id)
		if found != enabled {
			t.Fatalf("enabled=%v context=%v", enabled, context)
		}
		if enabled {
			encoded, err := json.Marshal(context)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), "stream original task") {
				t.Fatal("original input missing from replay")
			}
			next := map[string]any{"previous_response_id": id, "input": "next task"}
			if _, err := h.restoreContinuation(req, next); err != nil {
				t.Fatal(err)
			}
		} else if _, found := h.getResponseStore().get(owner, id); found {
			t.Fatal("private streamed response stored")
		}
	}
}

func TestStreamStorageFailureEndsWithFailedEvent(t *testing.T) {
	for _, text := range []string{"answer", `<tool_calls><invoke name="write"></invoke></tool_calls>`} {
		rec := httptest.NewRecorder()
		runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "resp", "model", "prompt", false, false, true, nil, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", func(map[string]any) bool { return false }, nil)
		runtime.accumulator.RawText.WriteString(text)
		runtime.accumulator.Text.WriteString(text)
		if !runtime.finalize("stop", false) || !runtime.failed {
			t.Fatal("storage failure did not fail the response")
		}
		body := rec.Body.String()
		if strings.Contains(body, "event: response.completed") || !strings.Contains(body, responseStorageCode) || strings.Contains(body, "function_call_arguments") || strings.Contains(body, `"type":"function_call"`) {
			t.Fatalf("incorrect terminal event or premature call delivery: %s", body)
		}
	}
}

func TestNonstreamStorageFailureDoesNotReturnSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStoredResponse(rec, map[string]any{"id": "resp"}, func(map[string]any) bool { return false }, nil, assistantturn.Turn{})
	if rec.Code != http.StatusInsufficientStorage || !strings.Contains(rec.Body.String(), responseStorageCode) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
