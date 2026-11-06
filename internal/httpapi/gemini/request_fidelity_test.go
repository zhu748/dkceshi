package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/promptcompat"
	"ds2api/internal/toolcall"
)

func TestGeminiTextPartsPreserveWhitespaceWithoutAddingSeparators(t *testing.T) {
	parts := []any{map[string]any{"text": "  code\n"}, map[string]any{"text": "\t next  "}}
	raw := map[string]any{"systemInstruction": map[string]any{"parts": parts}, "contents": []any{map[string]any{"role": "user", "parts": parts}}}
	messages := geminiMessagesFromRequest(raw)
	for _, item := range messages {
		if item.(map[string]any)["content"] != "  code\n\t next  " {
			t.Fatalf("text changed: %+v", item)
		}
	}
}

func TestGeminiThoughtAnnotationsReachPromptAndHistory(t *testing.T) {
	std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{"contents": []any{map[string]any{"role": "model", "parts": []any{map[string]any{"text": "  exact\r\n", "thought": true, "thoughtSignature": "OPAQUE_SIGNATURE", "annotations": []any{"THOUGHT_SOURCE"}}}}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
		if !strings.Contains(text, "THOUGHT_SOURCE") || !strings.Contains(text, "  exact\r\n") || strings.Contains(text, "OPAQUE_SIGNATURE") {
			t.Fatal("Gemini reasoning context was lost or leaked")
		}
	}
}

func TestGeminiOutputSchemaUsesSharedValidation(t *testing.T) {
	standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": "answer"}}}},
		"generationConfig": map[string]any{"responseMimeType": "application/json", "responseSchema": map[string]any{"type": "OBJECT", "required": []any{"value"}}},
	}, false)
	if err != nil || !standard.OutputFormat.Enabled() || !strings.Contains(standard.FinalPrompt, "Response format:") {
		t.Fatalf("schema not normalized: %+v err=%v", standard, err)
	}
	if standard.OutputFormat.Validate(`{"value":42}`) != nil || standard.OutputFormat.Validate(`{}`) == nil {
		t.Fatal("Gemini schema was not enforced")
	}
}

func TestGeminiNonStreamToolsKeepExplanation(t *testing.T) {
	parts := buildGeminiPartsFromTurn(assistantturn.Turn{Text: "before", ToolCalls: []toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{}}}})
	if len(parts) != 2 || parts[0]["text"] != "before" || parts[1]["functionCall"] == nil {
		t.Fatalf("prefix lost: %+v", parts)
	}
}
