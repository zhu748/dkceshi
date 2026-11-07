package prompt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNonObjectToolHistoryPreservesArgumentsAndOrdering(t *testing.T) {
	for _, arguments := range []any{nil, "null", "  \r\n", "false", "9007199254740993", `[]`, `"text"`, []any{false, "DATA"}} {
		calls := []any{
			map[string]any{"id": "before", "name": "first", "arguments": `{"key":"BEFORE"}`},
			map[string]any{"id": "middle", "function": map[string]any{"name": "second", "arguments": arguments}},
			map[string]any{"id": "after", "name": "third", "arguments": map[string]any{"key": "AFTER"}},
		}
		text := FormatToolCallsForPromptForMarker(calls, "PRIVATE_MARKER")
		start := strings.Index(text, opaqueToolHistoryLabel)
		if start < strings.Index(text, "BEFORE") || start > strings.Index(text, "AFTER") {
			t.Fatalf("history order changed: %s", text)
		}
		var record map[string]json.RawMessage
		line := strings.SplitN(text[start+len(opaqueToolHistoryLabel):], "\n", 2)[0]
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(arguments)
		if string(record["arguments"]) != string(want) || string(record["call_id"]) != `"middle"` {
			t.Fatalf("argument type/bytes or ID changed: %s", line)
		}
		if strings.Contains(text, "EPSE") || strings.Count(text, "<|PRIVATE_MARKER|tool_calls>") != 2 {
			t.Fatalf("abnormal history became an executable batch: %s", text)
		}
	}
}

func TestAbsentHistoryArgumentsStillRepresentAnEmptyObject(t *testing.T) {
	text := FormatToolCallsForPrompt([]any{map[string]any{"name": "noargs"}})
	if strings.Contains(text, opaqueToolHistoryLabel) || !strings.Contains(text, `invoke name="noargs"`) {
		t.Fatal(text)
	}
	value, present := ToolCallArguments(map[string]any{"arguments": nil, "function": map[string]any{"arguments": "{}"}})
	if !present || value != nil {
		t.Fatal("explicit null fell through to another alias")
	}
}
