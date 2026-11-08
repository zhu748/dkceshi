package gemini

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func TestGeminiSystemAttachmentsSurviveDirectAndChatBridgeUpload(t *testing.T) {
	for _, bridge := range []bool{false, true} {
		for _, key := range []string{"systemInstruction", "system_instruction"} {
			req := map[string]any{key: map[string]any{"parts": []any{map[string]any{"text": "  SYSTEM_CONTEXT\r\n", "attachments": []any{map[string]any{"type": "input_file", "file_data": "QUJDRA==", "filename": "requirements.txt", "description": "SYSTEM_FILE"}}}}, "files": []any{"existing-system-file"}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "CURRENT_TASK"}}}}}
			original, _ := json.Marshal(req)
			if bridge {
				std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
				if err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(req)
				if string(original) != string(after) {
					t.Fatal("adapter changed original system")
				}
				req = std.OpenAIChatRequest(req)
			}
			uploader := &contextMediaUploader{}
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
				std, err = normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(std.RefFileIDs) != 2 || !slices.Contains(std.RefFileIDs, "file-media-1") || !slices.Contains(std.RefFileIDs, "existing-system-file") {
				t.Fatalf("system refs missing: %v", std.RefFileIDs)
			}
			for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
				for _, want := range []string{"SYSTEM_CONTEXT", "CURRENT_TASK", "SYSTEM_FILE", "file-media-1", "existing-system-file"} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing %s", want)
					}
				}
				if strings.Contains(text, "QUJDRA==") {
					t.Fatal("system bytes leaked into prompt")
				}
			}
		}
	}
}

func TestGeminiSystemRejectsMixedPayloadBeforeUpload(t *testing.T) {
	req := map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "rules", "inlineData": map[string]any{"mimeType": "text/plain", "data": "QUJDRA=="}}}}, "contents": []any{map[string]any{"parts": []any{map[string]any{"text": "task"}}}}}
	if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
		t.Fatal("mixed system part accepted")
	}
}
