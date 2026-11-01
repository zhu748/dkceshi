package promptcompat

import "testing"

func historyCall(name, id string) map[string]any {
	return map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": id, "function": map[string]any{"name": name, "arguments": "{}"}}}}
}
func historyResult(name, id string) map[string]any {
	return map[string]any{"role": "tool", "name": name, "tool_call_id": id, "content": "result"}
}

func TestToolHistoryRejectsContradictionsAndAllowsPartialHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		messages []any
		invalid  bool
	}{
		{"duplicate pending ID", []any{historyCall("read", "a"), historyCall("write", "a")}, true},
		{"conflicting name", []any{historyCall("read", "a"), historyResult("write", "a")}, true},
		{"repeated result", []any{historyCall("read", "a"), historyResult("read", "a"), historyResult("read", "a")}, true},
		{"partial context", []any{historyResult("read", "external")}, false},
		{"out of order", []any{historyCall("read", "a"), historyCall("read", "b"), historyResult("read", "b"), historyResult("read", "a")}, false},
		{"completed ID reuse", []any{historyCall("read", "a"), historyResult("read", "a"), historyCall("write", "a"), historyResult("write", "a")}, false},
		{"reserved ID", []any{historyCall("read", ""), historyCall("read", "history_1")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateToolHistory(tc.messages); (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v err=%v", tc.invalid, err)
			}
			if !tc.invalid {
				return
			}
			req := map[string]any{"model": "deepseek-v4.1-flash", "messages": tc.messages}
			if _, err := NormalizeOpenAIChatRequest(nil, req, ""); err == nil {
				t.Fatal("Chat accepted contradictory history")
			}
			if _, err := NormalizeOpenAIResponsesRequest(nil, req, ""); err == nil {
				t.Fatal("Responses accepted contradictory history")
			}
		})
	}
}

func TestRejectedLinkDoesNotCorruptPendingQueue(t *testing.T) {
	links := NewToolCallLinker("call")
	links.Call("read", "a")
	links.Call("write", "a")
	if links.Err() == nil || links.names["a"] != "read" || len(links.pending["write"]) != 0 {
		t.Fatal("duplicate ID overwrote pending read")
	}
	id, name := links.Result("read", "")
	if id != "a" || name != "read" || len(links.pending["read"]) != 0 {
		t.Fatal("original call no longer matchable")
	}
}
