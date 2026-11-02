package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/toolcall"
)

func repairThinkingEvent(t *testing.T, channel, text string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"p": channel, "v": text})
	if err != nil {
		t.Fatal(err)
	}
	return "data: " + string(data)
}

func TestRepairEnablesThinkingAndKeepsItSeparateFromFinalOutput(t *testing.T) {
	const thought = `Check tag nesting first. <tool_calls><invoke name="wrong"></invoke></tool_calls>`
	const answer = `<tool_calls><invoke name="read"></invoke></tool_calls>`
	const prompt = "repair format only"
	store := newRepairHistoryStore(t)
	ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200,
		repairThinkingEvent(t, "response/thinking_content", thought),
		repairThinkingEvent(t, "response/content", answer), "data: [DONE]")}}
	output, err := NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, store)(context.Background(), prompt)
	if err != nil || output != answer {
		t.Fatalf("reasoning contaminated repair output: %q %v", output, err)
	}
	if len(ds.payloads) != 1 {
		t.Fatalf("unexpected completions: %d", len(ds.payloads))
	}
	payload := ds.payloads[0]
	if payload["thinking_enabled"] != true || payload["model_type"] != "default" {
		t.Fatalf("thinking not enabled: %+v", payload)
	}
	if payload["prompt"] != prompt || payload["parent_message_id"] != nil {
		t.Fatalf("repair inherited main context: %+v", payload)
	}
	if files, ok := payload["ref_file_ids"].([]any); !ok || len(files) != 0 {
		t.Fatalf("repair inherited files: %+v", payload)
	}
	entries := repairEntries(t, store)
	if len(entries) != 1 {
		t.Fatalf("unexpected history count: %d", len(entries))
	}
	entry, err := store.Get(entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ReasoningContent != thought || entry.Content != answer {
		t.Fatalf("reasoning and final text not archived separately: %+v", entry)
	}
}

func TestRepairDoesNotAcceptToolCallsFromThinkingOnly(t *testing.T) {
	const raw = `<|EPSE|call name="read"></|EPSE|call>`
	const thought = `<|EPSE|tool_calls><|EPSE|invoke name="read"></|EPSE|invoke></|EPSE|tool_calls>`
	ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200,
		repairThinkingEvent(t, "response/thinking_content", thought), "data: [DONE]")}}
	invoke := NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, nil)
	calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), raw, invoke)
	if ok || len(calls) != 0 {
		t.Fatalf("thinking-only candidate accepted as final repair: %+v", calls)
	}
	if len(ds.payloads) != 1 || ds.payloads[0]["thinking_enabled"] != true {
		t.Fatal("expected one thinking-enabled repair request")
	}
}
