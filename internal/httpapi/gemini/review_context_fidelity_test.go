package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiWhitespaceThoughtsKeepOrderAcrossChatBridge(t *testing.T) {
	raw := map[string]any{"contents": []any{map[string]any{"role": "model", "parts": []any{
		map[string]any{"thought": true, "text": " \r\n\t"},
		map[string]any{"text": "FIRST_TEXT"},
		map[string]any{"thought": true, "text": "\t \n"},
		map[string]any{"functionCall": map[string]any{"name": "lookup", "id": "call-1", "args": map[string]any{}}},
	}}}}
	native, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := promptcompat.NormalizeOpenAIChatRequest(nil, native.OpenAIChatRequest(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{native.FinalPrompt, bridge.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(bridge.Messages)} {
		previous := -1
		for _, want := range []string{"[reasoning_content]\n \r\n\t\n[/reasoning_content]", "FIRST_TEXT", "[reasoning_content]\n\t \n\n[/reasoning_content]", `call_id="call-1"`} {
			index := strings.Index(text, want)
			if index <= previous {
				t.Fatalf("missing or reordered native context %q: %q", want, text)
			}
			previous = index
		}
	}
}

func TestGeminiWhitespaceSystemInstructionIsNotRemoved(t *testing.T) {
	raw := map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": " \r\n\t"}}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}}
	standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", raw, false)
	if err != nil || !strings.Contains(standard.FinalPrompt, "[System]: \r\n\t\n") {
		t.Fatal("system instruction was silently omitted", err)
	}
}
