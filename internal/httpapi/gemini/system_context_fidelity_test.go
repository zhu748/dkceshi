package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiSystemTextFallbackRetainsContextMetadata(t *testing.T) {
	norm, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{
		"systemInstruction": map[string]any{"text": "  SYSTEM\r\n", "annotations": []any{"SYSTEM_SOURCE_SENTINEL"}},
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{norm.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(norm.Messages)} {
		if !strings.Contains(text, "  SYSTEM\r\n") || !strings.Contains(text, "SYSTEM_SOURCE_SENTINEL") {
			t.Fatalf("system text fallback lost context: %s", text)
		}
	}
}
