package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeExtensionsSurviveNativeToolsAndInternalChatBridge(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "user", "content": "TASK", "metadata": "USER_CONTEXT"},
		map[string]any{"role": "assistant", "metadata": "ASSISTANT_CONTEXT", "native_tool_context": "OUTER_NATIVE_CONTEXT", "content": []any{
			map[string]any{"type": "text", "text": "BEFORE"},
			map[string]any{"type": "tool_use", "id": "call", "name": "read", "input": map[string]any{}, "metadata": "CALL_CONTEXT"},
		}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call", "content": "RESULT", "metadata": "RESULT_CONTEXT"}}},
		map[string]any{"role": "assistant", "prefix": true, "content": []any{map[string]any{"type": "text", "text": "PREFIX"}}, "custom_context": "PREFILL_CONTEXT"},
	}}
	normalized, err := normalizeClaudeRequest(nil, req)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := promptcompat.NormalizeOpenAIChatRequest(nil, normalized.Standard.OpenAIChatRequest(req), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{normalized.Standard.FinalPrompt, bridge.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(normalized.Standard.Messages)} {
		for _, want := range []string{"USER_CONTEXT", "ASSISTANT_CONTEXT", "OUTER_NATIVE_CONTEXT", "CALL_CONTEXT", "RESULT_CONTEXT", "PREFILL_CONTEXT"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("extension missing or duplicated %s: %s", want, text)
			}
		}
	}
	if !strings.HasSuffix(bridge.FinalPrompt, "[Assistant]:PREFIX") {
		t.Fatal("native extension displaced prefill")
	}
}
