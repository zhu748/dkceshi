package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/promptcompat"
)

func TestClaudeProxyKeepsNestedFilesUntilSharedUpload(t *testing.T) {
	for _, toolResult := range []bool{false, true} {
		for _, kind := range []string{"document", "search_result"} {
			blocks := []any{map[string]any{"type": "text", "text": "  CONTEXT\r\n"}, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "QUJDRA=="}, "description": "NESTED_IMAGE"}}
			block := map[string]any{"type": kind, "title": "DOCUMENT_TITLE", "content": blocks}
			if kind == "document" {
				delete(block, "content")
				block["source"] = map[string]any{"type": "content", "content": blocks}
			}
			if toolResult {
				block = map[string]any{"type": "tool_result", "tool_use_id": "external-call", "content": []any{block}}
			}
			data, _ := json.Marshal(map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}})
			capture := &openAIProxyCaptureStub{}
			h := &Handler{OpenAI: capture}
			rec := httptest.NewRecorder()
			h.Messages(rec, httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(string(data))))
			if rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			uploader := &claudeCurrentInputDS{}
			if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, capture.seenReq, uploader, "deepseek-v4.1-flash", false); err != nil {
				t.Fatal(err)
			}
			if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "ABCD" {
				t.Fatalf("nested file disappeared before upload: %+v", uploader.uploads)
			}
			standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, capture.seenReq, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
				if !strings.Contains(text, "DOCUMENT_TITLE") || !strings.Contains(text, "NESTED_IMAGE") || !strings.Contains(text, "file-claude-history") || strings.Contains(text, "QUJDRA==") {
					t.Fatal("nested source context or reference was lost")
				}
			}
		}
	}
}

func TestClaudeProxyPreservesConflictingSourcesForUploadValidation(t *testing.T) {
	capture := &openAIProxyCaptureStub{}
	h := &Handler{OpenAI: capture}
	data := `{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"QQ=="},"file_data":"Qg=="}]}]}`
	rec := httptest.NewRecorder()
	h.Messages(rec, httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(data)))
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	uploader := &claudeCurrentInputDS{}
	if err := files.PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, capture.seenReq, uploader, "deepseek-v4.1-flash", false); err == nil || len(uploader.uploads) != 0 {
		t.Fatal("proxy discarded a byte alias before validation")
	}
}
