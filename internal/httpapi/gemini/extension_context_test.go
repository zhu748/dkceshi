package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiExtensionsSurviveNativeToolsAndInternalChatBridge(t *testing.T) {
	req := map[string]any{"contents": []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}, "metadata": "USER_CONTEXT"},
		map[string]any{"role": "model", "metadata": "ASSISTANT_CONTEXT", "native_tool_context": "OUTER_NATIVE_CONTEXT", "parts": []any{
			map[string]any{"functionCall": map[string]any{"id": "call", "name": "read", "args": map[string]any{}, "metadata": "CALL_CONTEXT"}, "metadata": "PART_CONTEXT"},
		}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"id": "call", "name": "read", "response": map[string]any{"text": "RESULT"}, "metadata": "RESULT_CONTEXT"}}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "PREFIX"}}, "prefix": true, "custom_context": "PREFILL_CONTEXT"},
	}}
	req["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": "SYSTEM_RULE"}}, "metadata": "SYSTEM_CONTEXT"}
	normalized, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := promptcompat.NormalizeOpenAIChatRequest(nil, normalized.OpenAIChatRequest(req), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{normalized.FinalPrompt, bridge.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(normalized.Messages)} {
		for _, want := range []string{"SYSTEM_CONTEXT", "USER_CONTEXT", "ASSISTANT_CONTEXT", "OUTER_NATIVE_CONTEXT", "CALL_CONTEXT", "PART_CONTEXT", "RESULT_CONTEXT", "PREFILL_CONTEXT"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("extension missing or duplicated %s: %s", want, text)
			}
		}
		if strings.Contains(text, `"parts"`) {
			t.Fatal("consumed protocol body was serialized twice")
		}
	}
	if !strings.HasSuffix(bridge.FinalPrompt, "[Assistant]:PREFIX") {
		t.Fatal("native extension displaced prefill")
	}
}
