package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/contextfile"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

type contextMediaUploader struct{ uploads []dsclient.UploadFileRequest }

func (u *contextMediaUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	u.uploads = append(u.uploads, req)
	return &dsclient.UploadFileResult{ID: fmt.Sprintf("file-media-%d", len(u.uploads))}, nil
}

type contextSplitEnabled struct{}

func (contextSplitEnabled) CurrentInputFileEnabled() bool { return true }
func (contextSplitEnabled) CurrentInputFileMinChars() int { return 1 }

func TestGeminiToolMediaSurvivesUploadNormalizationAndHistorySplit(t *testing.T) {
	var req map[string]any
	if err := json.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"inspect images"}]},{"role":"model","parts":[{"functionCall":{"id":"c1","name":"get_image","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"id":"c1","name":"get_image","response":{"image":{"$ref":"photo.png"},"status":"success"},"parts":[{"inlineData":{"mimeType":"image/png","data":"QUJDRA==","displayName":"photo.png"}}]}}]},{"role":"user","parts":[{"text":"CURRENT_TASK"}]}]}`), &req); err != nil {
		t.Fatal(err)
	}
	uploader := &contextMediaUploader{}
	a := &auth.RequestAuth{}
	if err := files.PreprocessInlineFiles(context.Background(), a, req, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{std.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(std.Messages)} {
		for _, want := range []string{"photo.png", "file-media-1", `"$ref"`, `call_id="c1"`, "success"} {
			if !strings.Contains(text, want) {
				t.Fatalf("normalized context lost %q: %s", want, text)
			}
		}
		if strings.Contains(text, "QUJDRA==") {
			t.Fatal("media bytes leaked into text")
		}
	}
	split, err := (contextfile.Service{Store: contextSplitEnabled{}, DS: uploader}).ApplyCurrentInputFile(context.Background(), a, std)
	if err != nil || !split.CurrentInputFileApplied {
		t.Fatalf("history split failed: %v", err)
	}
	if !strings.Contains(split.HistoryText, "file-media-1") || !strings.Contains(split.HistoryText, "photo.png") || !strings.Contains(split.FinalPrompt, "CURRENT_TASK") {
		t.Fatal("tool media lost when moving history into a file")
	}
	if len(split.RefFileIDs) != 2 {
		t.Fatalf("history replaced tool media reference: %v", split.RefFileIDs)
	}
}
