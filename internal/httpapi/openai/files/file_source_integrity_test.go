package files

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestInlineSourceConflictsFailBeforeUploadingTheFile(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "input_file", "file_data": "data:text/plain;base64,QQ==", "data": "Qg=="},
		{"type": "input_file", "file_data": "QQ==", "file": map[string]any{"file_data": "Qg=="}},
		{"type": "input_image", "image_url": "data:image/png;base64,QQ==", "inlineData": map[string]any{"mimeType": "image/png", "data": "Qg=="}},
		{"inlineData": map[string]any{"mimeType": "text/plain", "data": "QQ=="}, "inline_data": map[string]any{"mimeType": "text/plain", "data": "Qg=="}},
		{"type": "input_file", "file_data": "data:text/plain;base64,QQ==", "url": "data:application/pdf;base64,QQ=="},
		{"type": "input_file", "file_data": "QQ==", "data": nil},
		{"type": "input_file", "file_data": "QQ==", "file": false},
		{"type": "input_file", "file": map[string]any{"file_url": "https://example.test/source.txt"}},
	} {
		request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		before, _ := json.Marshal(request)
		uploader := &nativeFileUploader{}
		err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false)
		if status, _ := InlineFileErrorDetails(err); err == nil || status != 400 || len(uploader.uploads) != 0 {
			t.Fatalf("conflicting sources reached upload: err=%v uploads=%d", err, len(uploader.uploads))
		}
		after, _ := json.Marshal(request)
		if string(before) != string(after) || strings.Contains(err.Error(), "QQ==") || strings.Contains(err.Error(), "Qg==") {
			t.Fatal("failed validation changed source or disclosed bytes")
		}
	}
}

func TestEquivalentFileSourcesAndUploadedReferencesRemainUsable(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "input_file", "file_data": "data:text/plain;base64,QQ==", "base64": "QQ", "data": "QQ=="},
		{"type": "input_file", "file_data": "data:text/plain;base64,QQ==", "file": map[string]any{"file_data": "QQ=="}},
		{"type": "input_file", "file": map[string]any{"file_url": "data:text/plain;base64,QQ=="}},
		{"inlineData": map[string]any{"mimeType": "text/plain", "data": "QQ=="}, "inline_data": map[string]any{"mime_type": "text/plain", "data": "QQ"}},
	} {
		decoded, matched, err := decodeOpenAIInlineFileBlock(block)
		if err != nil || !matched || string(decoded.Data) != "A" {
			t.Fatalf("equivalent sources rejected: %v %+v", err, decoded)
		}
	}
	for _, block := range []map[string]any{
		{"type": "input_file", "file_id": "file-existing", "file_data": "old", "data": false},
		{"type": "input_file", "file": map[string]any{"file_id": "file-existing"}, "file_data": "old", "data": false},
	} {
		if _, matched, err := decodeOpenAIInlineFileBlock(block); err != nil || matched {
			t.Fatal("uploaded reference was forced through obsolete source data")
		}
	}
}

func TestMessageOwnedFilesUploadAndRenderOnce(t *testing.T) {
	for _, role := range []string{"user", "tool"} {
		for _, key := range promptcompat.AttachmentInputKeys() {
			request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": role, "content": "BODY", key: []any{
				map[string]any{"type": "input_file", "filename": "source.txt", "description": "MESSAGE_ATTACHMENT", "file_data": "data:text/plain;base64,QQ=="},
			}}}}
			uploader := &nativeFileUploader{}
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil {
				t.Fatal(err)
			}
			standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
			if err != nil || len(uploader.uploads) != 1 || len(standard.RefFileIDs) != 1 || strings.Count(standard.FinalPrompt, "MESSAGE_ATTACHMENT") != 1 || strings.Contains(standard.FinalPrompt, "QQ==") {
				t.Fatalf("message attachment incomplete: err=%v prompt=%s", err, standard.FinalPrompt)
			}
		}
	}
}

func TestHistoricalNativeArgumentsAreNeverUploadedOrCollectedAsFiles(t *testing.T) {
	business := map[string]any{"type": "input_file", "file_id": "BUSINESS_ID", "file_data": "BUSINESS_BYTES", "name": "record.txt"}
	for _, request := range []map[string]any{
		{"messages": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "c", "name": "read", "input": business}}}}},
		{"contents": []any{map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": "c", "name": "read", "args": business}}}}}},
		{"input": []any{map[string]any{"type": "function_call", "name": "read", "input": business}}},
	} {
		before, _ := json.Marshal(request)
		uploader := &nativeFileUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil || len(uploader.uploads) != 0 || len(promptcompat.CollectOpenAIRefFileIDs(request)) != 0 {
			t.Fatalf("historical tool arguments became attachments: %v", err)
		}
		source := map[string]any{}
		for _, key := range []string{"messages", "contents", "input"} {
			if value, present := request[key]; present {
				source[key] = value
			}
		}
		after, _ := json.Marshal(source)
		if string(before) != string(after) {
			t.Fatal("file preprocessing rewrote historical business arguments")
		}
	}
}

func TestNativeToolItemAttachmentsRemainInUploadDiscovery(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		request := map[string]any{"messages": []any{map[string]any{"role": "assistant", "content": []any{map[string]any{
			"type": "tool_use", "id": "c", "name": "read", "input": map[string]any{"file_id": "BUSINESS_ID", "file_data": "NOT_A_FILE"},
			key: []any{map[string]any{"type": "input_file", "filename": "native.txt", "file_data": "data:text/plain;base64,QQ=="}},
		}}}}}
		uploader := &nativeFileUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil || len(uploader.uploads) != 1 {
			t.Fatalf("native tool attachments were skipped: %v", err)
		}
		refs := promptcompat.CollectOpenAIRefFileIDs(request)
		if len(refs) != 1 || refs[0] == "BUSINESS_ID" {
			t.Fatal("native tool attachment identity mixed with its arguments")
		}
	}
}
