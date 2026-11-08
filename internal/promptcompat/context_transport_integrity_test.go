package promptcompat_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/contextfile"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

type integrityContextConfig struct{}

func (integrityContextConfig) CurrentInputFileEnabled() bool { return true }
func (integrityContextConfig) CurrentInputFileMinChars() int { return 1 }

type integrityContextUploader struct{ history string }

func (u *integrityContextUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, file dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if file.Filename == promptcompat.CurrentInputContextFilename {
		u.history = string(file.Data)
	}
	return &dsclient.UploadFileResult{ID: "generated-" + file.Filename}, nil
}

func TestBusinessContextSurvivesUploadNormalizationAndExternalization(t *testing.T) {
	for _, surface := range []string{"chat", "responses"} {
		t.Run(surface, func(t *testing.T) {
			const marker = "Q7ZK3M"
			links := []any{map[string]any{"url": "https://example.com/report", "title": "HISTORICAL_RESULT", "data": "PLAIN_BUSINESS_DATA"}}
			old := map[string]any{"role": "user", "content": "EARLIER_USER_TASK"}
			current := map[string]any{"role": "user", "id": "latest-user", "content": []any{
				map[string]any{"type": "text", "text": "  LATEST_USER_TASK\n"},
				map[string]any{"type": "image_url", "image_url": "data:image/png;base64,QUJDRA==", "description": "CURRENT_IMAGE"},
			}}
			req := map[string]any{"model": "deepseek-v4.1-flash"}
			if surface == "chat" {
				req["messages"] = []any{old,
					map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "search-call", "name": "search", "arguments": map[string]any{"query": "HISTORICAL_QUERY"}}}},
					map[string]any{"role": "tool", "tool_call_id": "search-call", "content": links}, current}
			} else {
				req["input"] = []any{old,
					map[string]any{"type": "function_call", "call_id": "search-call", "name": "search", "arguments": `{"query":"HISTORICAL_QUERY"}`},
					map[string]any{"type": "function_call_output", "call_id": "search-call", "output": links}, current}
			}
			promptcompat.CaptureInputFieldDigests(req)
			source := promptcompat.InputFieldDigests(req)
			route := promptcompat.PlanImageRoute(req, nil)
			uploader := &contextTransportUploader{}
			if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", true); err != nil {
				t.Fatal(err)
			}
			route.StripImages(req)
			var normalized promptcompat.StandardRequest
			var err error
			if surface == "chat" {
				normalized, err = promptcompat.NormalizeOpenAIChatRequest(nil, req, "", marker)
			} else {
				normalized, err = promptcompat.NormalizeOpenAIResponsesRequest(nil, req, "", marker)
			}
			if err != nil {
				t.Fatal(err)
			}
			if uploader.uploads != 1 || !reflect.DeepEqual(source, normalized.InputContext) {
				t.Fatal("upload count or raw source evidence changed")
			}
			archive := promptcompat.BuildOpenAIHistoryTranscript(normalized.Messages, marker)
			for _, text := range []string{normalized.FinalPrompt, archive} {
				for _, expected := range []string{"HISTORICAL_RESULT", "HISTORICAL_QUERY", "PLAIN_BUSINESS_DATA", "https://example.com/report", "search-call", "  LATEST_USER_TASK\n", "CURRENT_IMAGE", "file-image"} {
					if !strings.Contains(text, expected) {
						t.Fatalf("normalized context missing %q", expected)
					}
				}
				if strings.Contains(text, "QUJDRA==") {
					t.Fatal("raw image bytes entered conversation text")
				}
			}
			contextUploader := &integrityContextUploader{}
			service := contextfile.Service{Store: integrityContextConfig{}, DS: contextUploader}
			split, err := service.ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, normalized)
			if err != nil {
				t.Fatal(err)
			}
			if !split.CurrentInputFileApplied || split.HistoryText != contextUploader.history {
				t.Fatal("history text and the uploaded file diverged")
			}
			for _, expected := range []string{"HISTORICAL_RESULT", "HISTORICAL_QUERY", "PLAIN_BUSINESS_DATA", "https://example.com/report", "search-call"} {
				if !strings.Contains(split.HistoryText, expected) {
					t.Fatalf("externalized context missing %q", expected)
				}
			}
			if !strings.Contains(split.FinalPrompt, "  LATEST_USER_TASK\n") || !strings.Contains(split.FinalPrompt, "CURRENT_IMAGE") || len(split.RefFileIDs) != 2 {
				t.Fatal("history externalization detached current input or its image")
			}
		})
	}
}
