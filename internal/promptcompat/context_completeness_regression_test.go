package promptcompat_test

import (
	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
	"strings"
	"testing"
)

func TestRegressionChatAssistantAudioContext(t *testing.T) {
	raw := []any{map[string]any{"role": "assistant", "content": nil, "audio": map[string]any{"id": "aud_1", "transcript": "AUDIO_TRANSCRIPT_SENTINEL"}}, map[string]any{"role": "user", "content": "repeat that"}}
	text, _ := promptcompat.BuildOpenAIPrompt(raw, nil, "", promptcompat.DefaultToolChoicePolicy(), true, promptcompat.ToolReminderConfig{})
	if !strings.Contains(text, "AUDIO_TRANSCRIPT_SENTINEL") {
		t.Fatal("assistant audio transcript missing from final prompt")
	}
}

func TestRegressionAnnotatedTextContext(t *testing.T) {
	text := prompt.NormalizeContent([]any{map[string]any{"type": "output_text", "text": "See reference.", "annotations": []any{map[string]any{"type": "url_citation", "url": "https://example.com/SOURCE_SENTINEL", "title": "source"}}}})
	if !strings.Contains(text, "SOURCE_SENTINEL") {
		t.Fatal("text annotation/citation lost: " + text)
	}
}

func TestRegressionResponsesSearchHistory(t *testing.T) {
	messages := promptcompat.NormalizeResponsesInputAsMessages([]any{map[string]any{"type": "file_search_call", "id": "fs_1", "status": "completed", "queries": []any{"QUERY_SENTINEL"}, "results": []any{map[string]any{"text": "RESULT_SENTINEL", "filename": "a.txt"}}}, map[string]any{"role": "user", "content": "summarize the result"}})
	text := promptcompat.BuildOpenAIHistoryTranscript(messages)
	if !strings.Contains(text, "RESULT_SENTINEL") {
		t.Fatal("Responses built-in tool history item dropped")
	}
}

func TestRegressionCustomToolForcedChoice(t *testing.T) {
	_, err := promptcompat.ParseOpenAIToolPolicy(map[string]any{"tools": []any{map[string]any{"type": "custom", "name": "code_exec"}}, "tool_choice": map[string]any{"type": "custom", "name": "code_exec"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegressionUnknownTextBlockData(t *testing.T) {
	text := prompt.NormalizeContent([]any{map[string]any{"type": "tool_response", "data": "PLAIN_RESULT_SENTINEL"}})
	if !strings.Contains(text, "PLAIN_RESULT_SENTINEL") {
		t.Fatal("plain data replaced: " + text)
	}
}
