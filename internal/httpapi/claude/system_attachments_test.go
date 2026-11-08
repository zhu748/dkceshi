package claude

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func TestClaudeSystemAttachmentsSurviveDirectAndChatBridgeUpload(t *testing.T) {
	for _, bridge := range []bool{false, true} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "system": []any{map[string]any{"type": "text", "text": "  SYSTEM_CONTEXT\r\n", "attachments": []any{map[string]any{"type": "input_file", "file_data": "QUJDRA==", "filename": "requirements.txt", "description": "SYSTEM_FILE"}}}}, "messages": []any{map[string]any{"role": "user", "content": "CURRENT_TASK"}}}
		if bridge {
			norm, err := normalizeClaudeRequest(nil, req)
			if err != nil {
				t.Fatal(err)
			}
			req = norm.Standard.OpenAIChatRequest(req)
		}
		uploader := &claudeCurrentInputDS{}
		if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
			t.Fatalf("system bytes lost: %+v", uploader.uploads)
		}
		var std promptcompat.StandardRequest
		var err error
		if bridge {
			std, err = promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
		} else {
			var norm claudeNormalizedRequest
			norm, err = normalizeClaudeRequest(nil, req)
			std = norm.Standard
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(std.RefFileIDs) != 1 || std.RefFileIDs[0] != "file-claude-history" {
			t.Fatalf("missing system refs: %v", std.RefFileIDs)
		}
		for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
			for _, want := range []string{"  SYSTEM_CONTEXT\r\n", "SYSTEM_FILE", "file-claude-history", "CURRENT_TASK"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s", want)
				}
			}
			if strings.Contains(text, "QUJDRA==") {
				t.Fatal("system bytes leaked")
			}
		}
	}
}
