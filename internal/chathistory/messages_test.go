package chathistory

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureMessagesPersistsToolOnlyTurnsMetadataAndIndependentSnapshots(t *testing.T) {
	call := map[string]any{"id": "call_a", "type": "function", "function": map[string]any{"name": "lookup", "arguments": map[string]any{"id": json.Number("9007199254740993")}}}
	messages := CaptureMessages([]any{
		map[string]any{"role": "assistant", "content": nil, "reasoning_content": "thinking", "tool_calls": []any{call}},
		map[string]any{"role": "tool", "name": "lookup", "tool_call_id": "call_a", "content": "", "is_error": true},
		map[string]any{"role": "user", "content": "  keep whitespace\n"},
	})
	call["id"] = "mutated"
	if len(messages) != 3 || !strings.Contains(string(messages[0].ToolCalls), "9007199254740993") || strings.Contains(string(messages[0].ToolCalls), "mutated") || messages[1].ToolCallID != "call_a" || messages[1].IsError == nil || !*messages[1].IsError || messages[2].Content != "  keep whitespace\n" {
		t.Fatalf("captured messages lost fidelity: %#v", messages)
	}
	path := filepath.Join(t.TempDir(), "history.json")
	store := New(path)
	tools := json.RawMessage(`[{"name":"lookup"}]`)
	entry, err := store.Start(StartParams{Messages: messages, Tools: tools, ToolChoice: json.RawMessage(`{"Mode":"auto"}`), ToolMarker: "Q7ZK3M"})
	if err != nil {
		t.Fatal(err)
	}
	tools[0] = '!'
	messages[0].ToolCalls[0] = '!'
	*messages[1].IsError = false
	entry.Messages[0].ToolCalls[0] = '!'
	*entry.Messages[1].IsError = false
	got, err := store.Get(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Tools[0] = '!'
	if got.Messages[0].ToolCalls[0] != '[' || !*got.Messages[1].IsError {
		t.Fatalf("caller mutation corrupted store: %#v", got)
	}
	reloaded, err := New(path).Get(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, reloaded.Tools); err != nil {
		t.Fatal(err)
	}
	if compact.String() != `[{"name":"lookup"}]` || reloaded.ToolMarker != "Q7ZK3M" || reloaded.Messages[0].ToolCalls[0] != '[' || !*reloaded.Messages[1].IsError {
		t.Fatalf("round trip lost tool diagnostics: %#v", reloaded)
	}
}
