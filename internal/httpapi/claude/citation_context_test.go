package claude

import (
	"ds2api/internal/promptcompat"
	"strings"
	"testing"
)

func TestClaudeCitationsAndUnknownTextSurvivePromptAndHistory(t *testing.T) {
	quote := strings.Repeat("原始引用。", 2000)
	raw := []any{map[string]any{"role": "assistant", "content": []any{
		map[string]any{"type": "text", "text": "Answer.", "citations": []any{map[string]any{"type": "char_location", "document_index": 0, "cited_text": quote}}},
		map[string]any{"type": "future_context", "data": "BUSINESS_TEXT_SENTINEL", "text": strings.Repeat("A", 1500)},
	}}}
	messages := normalizeClaudeMessages(raw)
	text, _ := promptcompat.BuildOpenAIPrompt(messages, nil, "", promptcompat.DefaultToolChoicePolicy(), true, promptcompat.ToolReminderConfig{})
	for _, rendered := range []string{text, promptcompat.BuildOpenAIHistoryTranscript(messages)} {
		if !strings.Contains(rendered, quote) || !strings.Contains(rendered, "BUSINESS_TEXT_SENTINEL") || !strings.Contains(rendered, strings.Repeat("A", 1500)) {
			t.Fatal("text/citations lost in conversion")
		}
	}
}
