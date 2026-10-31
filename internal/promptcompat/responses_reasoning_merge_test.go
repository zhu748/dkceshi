package promptcompat

import (
	"strings"
	"testing"
)

func TestMergedToolCallsRetainBothReasoningBlocks(t *testing.T) {
	input := []any{
		map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "reasoning-first"}}},
		map[string]any{"type": "function_call", "name": "first", "call_id": "a", "arguments": "{}"},
		map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "reasoning-second"}}},
		map[string]any{"type": "function_call", "name": "second", "call_id": "b", "arguments": "{}"},
	}
	got := BuildOpenAIHistoryTranscript(NormalizeResponsesInputAsMessages(input))
	if !strings.Contains(got, "reasoning-first") || !strings.Contains(got, "reasoning-second") {
		t.Errorf("tool-call merge silently dropped reasoning: %s", got)
	}
}
