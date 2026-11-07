package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeBlockAdapterPreservesOuterAssistantContext(t *testing.T) {
	for _, standalone := range []string{"  OUTER_REASON\r\n", "BLOCK_A\nBLOCK_B"} {
		message := map[string]any{"role": "assistant", "reasoning_content": standalone, "refusal": "OUTER_REFUSAL", "audio": map[string]any{"id": "audio-ref", "transcript": "OUTER_TRANSCRIPT"}, "content": []any{
			map[string]any{"type": "text", "text": "BODY_FIRST"}, map[string]any{"type": "thinking", "thinking": "BLOCK_A"}, map[string]any{"type": "text", "text": "BODY_MIDDLE"}, map[string]any{"type": "thinking", "thinking": "BLOCK_B"},
		}}
		norm, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}})
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{norm.Standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(norm.Standard.Messages)} {
			for _, want := range []string{"BLOCK_A", "BLOCK_B", "OUTER_REFUSAL", "OUTER_TRANSCRIPT", "audio-ref"} {
				if strings.Count(text, want) != 1 {
					t.Fatalf("source=%q lost/duplicated %s: %s", standalone, want, text)
				}
			}
			if strings.Contains(standalone, "OUTER_REASON") && (!strings.Contains(text, standalone) || strings.Index(text, "OUTER_REASON") > strings.Index(text, "BODY_FIRST")) {
				t.Fatal("standalone reasoning lost its original leading position")
			}
		}
	}
}

func TestClaudeOuterContextIsValidatedBeforeFlattening(t *testing.T) {
	for _, extra := range []map[string]any{{"reasoning_content": 1}, {"refusal": []any{"bad"}}, {"audio": "bad"}, {"audio": map[string]any{"transcript": 1}}} {
		message := map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "valid neighbor"}}}
		for key, value := range extra {
			message[key] = value
		}
		if _, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}}); err == nil {
			t.Fatalf("invalid context disappeared: %+v", extra)
		}
	}
}
