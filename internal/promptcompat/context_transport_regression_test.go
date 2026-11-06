package promptcompat_test

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

type contextTransportUploader struct{ uploads int }

func (u *contextTransportUploader) UploadFile(context.Context, *auth.RequestAuth, dsclient.UploadFileRequest, int) (*dsclient.UploadFileResult, error) {
	u.uploads++
	return &dsclient.UploadFileResult{ID: "file-image"}, nil
}

func TestImagePreprocessingRetainsAssistantContextAndBoundaries(t *testing.T) {
	for _, history := range []map[string]any{
		{"role": "assistant", "content": nil, "refusal": "REFUSAL_SENTINEL"},
		{"role": "assistant", "content": "", "audio": map[string]any{"id": "AUDIO_ID", "transcript": "TRANSCRIPT_SENTINEL"}},
		{"role": "assistant", "content": []any{}, "id": "MESSAGE_ID", "prefix": false},
	} {
		before := promptcompat.BuildOpenAIHistoryTranscript([]any{history})
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{history,
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="}}}},
		}}
		route := promptcompat.PlanImageRoute(req, nil)
		uploader := &contextTransportUploader{}
		if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{AccountID: "test"}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		route.StripImages(req)
		std, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil {
			t.Fatal(err)
		}
		if uploader.uploads != 1 || len(std.Messages) != 2 || len(std.RefFileIDs) != 1 {
			t.Fatalf("transport changed message/file identity: %+v", std)
		}
		after := promptcompat.BuildOpenAIHistoryTranscript(std.Messages[:1])
		if before != after {
			t.Fatalf("image processing rewrote unrelated history:\nbefore %s\nafter %s", before, after)
		}
		for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
			if !strings.Contains(text, "file-image") || strings.Contains(text, "QUJDRA==") {
				t.Fatal("uploaded image reference missing or raw bytes exposed")
			}
		}
	}
}

func TestResponsesCallAliasesRetainResultDependencies(t *testing.T) {
	for _, alias := range []string{"call_id", "tool_call_id", "id"} {
		call := map[string]any{"type": "function_call", "name": "read", "arguments": "{}", alias: "original-call"}
		req := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
			map[string]any{"role": "user", "content": "old task"}, call,
			map[string]any{"role": "user", "content": "continue"},
			map[string]any{"type": "function_call_output", "call_id": "original-call", "output": "RESULT_SENTINEL"},
		}}
		std, err := promptcompat.NormalizeOpenAIResponsesRequest(nil, req, "")
		if err != nil {
			t.Fatal(err)
		}
		history, live := promptcompat.SplitActiveContext(std.Messages)
		if len(history) != 1 || len(live) != 3 {
			t.Fatalf("%s split detached the result from its call: history=%v live=%v", alias, history, live)
		}
		for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
			if !strings.Contains(text, `call_id="original-call"`) || !strings.Contains(text, "RESULT_SENTINEL") {
				t.Fatalf("%s lost linked history: %s", alias, text)
			}
		}
	}
}
