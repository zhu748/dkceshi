package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiRequestAttachmentsSurviveChatBridgeOnce(t *testing.T) {
	raw := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}, "files": []any{map[string]any{"file_id": "f", "filename": "NAME.txt", "description": "ATTACHMENT_CONTEXT"}}}
	standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", raw, false)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := promptcompat.NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(raw), "")
	if err != nil || strings.Count(chat.FinalPrompt, "ATTACHMENT_CONTEXT") != 1 || !strings.Contains(chat.FinalPrompt, "NAME.txt") {
		t.Fatalf("attachment lost or duplicated across Gemini bridge: %v %s", err, chat.FinalPrompt)
	}
}
