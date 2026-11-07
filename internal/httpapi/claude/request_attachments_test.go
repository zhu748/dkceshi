package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeRequestAttachmentsSurviveChatBridgeOnce(t *testing.T) {
	raw := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "TASK"}}, "attachments": []any{map[string]any{"file_id": "f", "filename": "NAME.txt", "description": "ATTACHMENT_CONTEXT"}}}
	standard, err := normalizeClaudeRequest(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := promptcompat.NormalizeOpenAIChatRequest(nil, standard.Standard.OpenAIChatRequest(raw), "")
	if err != nil || strings.Count(chat.FinalPrompt, "ATTACHMENT_CONTEXT") != 1 || !strings.Contains(chat.FinalPrompt, "NAME.txt") {
		t.Fatalf("attachment lost or duplicated across Claude bridge: %v %s", err, chat.FinalPrompt)
	}
}
