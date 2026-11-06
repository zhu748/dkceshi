package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiTextMetadataUsesSharedContextRendering(t *testing.T) {
	part := map[string]any{"text": "  BODY\n", "citations": []any{map[string]any{"title": "TEXT_CITATION"}}, "thoughtSignature": "OPAQUE_SECRET"}
	req := map[string]any{"systemInstruction": map[string]any{"parts": []any{part}}, "contents": []any{map[string]any{"role": "user", "parts": []any{part}}}}
	messages := geminiMessagesFromRequest(req)
	for _, text := range []string{promptcompat.RenderPromptText(messages, false), promptcompat.BuildOpenAIHistoryTranscript(messages)} {
		if strings.Count(text, "TEXT_CITATION") != 2 || strings.Count(text, "  BODY\n") != 2 || strings.Contains(text, "OPAQUE_SECRET") {
			t.Fatalf("Gemini text metadata changed: %s", text)
		}
	}
	if part["type"] != nil {
		t.Fatal("normalization mutated the original part")
	}
}
