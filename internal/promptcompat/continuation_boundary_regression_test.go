package promptcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestReferenceCollectionDoesNotTreatConversationDataAsFileIDs(t *testing.T) {
	req := map[string]any{"ref_file_ids": []any{"real-a"}, "attachments": []any{"real-b", map[string]any{"file_id": "real-c"}}, "messages": []any{
		map[string]any{"role": "user", "content": []any{"literal string", map[string]any{"type": "profile", "id": "not-a-file"}, map[string]any{"type": "input_file", "file_id": "real-d"}}},
		map[string]any{"role": "tool", "content": []any{"line one", "line two"}},
		map[string]any{"role": "tool", "content": map[string]any{"file_id": "database-row", "file": map[string]any{"id": "table-id"}}},
	}}
	if got := CollectOpenAIRefFileIDs(req); !reflect.DeepEqual(got, []string{"real-a", "real-b", "real-c", "real-d"}) {
		t.Fatalf("conversation text became a file reference: %v", got)
	}
}

func TestResponsesReasoningDoesNotEraseRefusalOrExplicitMessageBoundary(t *testing.T) {
	for _, message := range []map[string]any{
		{"role": "assistant", "refusal": "REFUSAL", "reasoning_content": "WHY"},
		{"role": "assistant", "content": "", "reasoning_content": "WHY", "prefix": true},
	} {
		got := NormalizeResponsesInputAsMessages([]any{message, map[string]any{"type": "function_call", "name": "check", "call_id": "c1", "arguments": "{}"}})
		if len(got) != 2 {
			t.Fatalf("explicit assistant message merged: %#v", got)
		}
		first := got[0].(map[string]any)
		if message["refusal"] != nil && first["refusal"] != message["refusal"] {
			t.Fatal("refusal was erased by reasoning merge")
		}
		if message["prefix"] != nil && first["prefix"] != true {
			t.Fatal("explicit prefill boundary lost")
		}
	}
}

func TestResponsesFallbackAndReasoningStayChronological(t *testing.T) {
	got := NormalizeResponsesInputAsMessages([]any{"  FIRST  ", map[string]any{"type": "reasoning", "text": "SECOND"}, map[string]any{"role": "user", "content": "THIRD"}})
	if len(got) != 3 || got[0].(map[string]any)["content"] != "  FIRST  " || got[1].(map[string]any)["reasoning_content"] != "SECOND" || got[2].(map[string]any)["content"] != "THIRD" {
		t.Fatalf("message order or whitespace changed: %#v", got)
	}
	transcript := BuildOpenAIHistoryTranscript(got)
	if strings.Index(transcript, "FIRST") > strings.Index(transcript, "SECOND") {
		t.Fatal("history chronology changed")
	}
}
