package claude

import (
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeArrayTextKeepsMessageIdentityAndPrefill(t *testing.T) {
	var previous string
	for _, content := range []any{"  PREFILL\r\n", []any{map[string]any{"type": "text", "text": "  PREFILL\r\n"}}} {
		message := map[string]any{"role": "assistant", "id": "message", "name": "speaker", "prefix": true, "content": content}
		normalized := normalizeClaudeMessages([]any{message})
		text, _ := promptcompat.BuildOpenAIPrompt(normalized, nil, "", promptcompat.DefaultToolChoicePolicy(), false, promptcompat.ToolReminderConfig{})
		if !strings.HasSuffix(text, "[Assistant]:  PREFILL\r\n") || !strings.Contains(text, `"id":"message"`) || !strings.Contains(text, `"name":"speaker"`) {
			t.Fatalf("prefill/identity lost: %s", text)
		}
		if previous != "" && previous != text {
			t.Fatal("string and array bodies render differently")
		}
		previous = text
	}
}

func TestClaudeSplitBlocksAssignMetadataWithoutMovingThinking(t *testing.T) {
	message := map[string]any{"role": "assistant", "id": "source", "name": "speaker", "prefix": true, "content": []any{
		map[string]any{"type": "text", "text": "BEFORE"}, map[string]any{"type": "thinking", "thinking": "THINK"}, map[string]any{"type": "text", "text": "AFTER"},
	}}
	normalized := normalizeClaudeMessages([]any{message})
	if len(normalized) != 2 || normalized[0].(map[string]any)["id"] != "source" || normalized[0].(map[string]any)["prefix"] != nil || normalized[1].(map[string]any)["prefix"] != true {
		t.Fatalf("metadata assigned to wrong fragment: %+v", normalized)
	}
	if !reflect.DeepEqual(message["prefix"], true) || message["content"].([]any)[0].(map[string]any)["text"] != "BEFORE" {
		t.Fatal("source message mutated")
	}
}
