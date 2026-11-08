package files

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestNativeBridgeCannotDiscardConflictingByteSources(t *testing.T) {
	for _, native := range []map[string]any{
		{"type": "document", "source": map[string]any{"type": "base64", "media_type": "text/plain", "data": "QQ=="}, "file_data": "Qg=="},
		{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "QQ=="}, "image_url": "data:image/png;base64,Qg=="},
		{"type": "image", "source": map[string]any{"type": "url", "url": "data:image/png;base64,QQ=="}, "image_url": "data:image/png;base64,Qg=="},
	} {
		for _, convertFirst := range []bool{false, true} {
			block := native
			if convertFirst {
				var ok bool
				block, ok = promptcompat.NormalizeAttachmentContent(native)
				if !ok {
					t.Fatal("native attachment was not recognized")
				}
			}
			request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
			uploader := &nativeFileUploader{}
			err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false)
			if err == nil || len(uploader.uploads) != 0 {
				t.Fatalf("convertFirst=%v lost a conflicting byte source: err=%v uploads=%d", convertFirst, err, len(uploader.uploads))
			}
		}
	}
}

func TestMissingOrMalformedAttachmentsNeverBecomeEmptyTasks(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "image"}, {"type": "input_image"}, {"type": "image_url", "image_url": map[string]any{}},
		{"type": "document"}, {"type": "input_file"}, {"type": "file_reference"},
		{"type": "image", "source": nil}, {"type": "document", "source": "not-an-object"},
		{"type": "image", "source": map[string]any{"type": "unknown", "data": "PRIVATE_SOURCE"}},
		{"inlineData": "PRIVATE_SOURCE"},
	} {
		for _, convertFirst := range []bool{false, true} {
			converted := block
			if convertFirst {
				var ok bool
				converted, ok = promptcompat.NormalizeAttachmentContent(block)
				if !ok {
					t.Fatal("invalid attachment disappeared before shared validation")
				}
			}
			uploader := &nativeFileUploader{}
			request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{converted}}}}
			err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false)
			status, _ := InlineFileErrorDetails(err)
			if err == nil || status != 400 || len(uploader.uploads) != 0 || strings.Contains(err.Error(), "PRIVATE_SOURCE") {
				t.Fatalf("invalid attachment became an empty task: err=%v uploads=%d", err, len(uploader.uploads))
			}
		}
	}
}

func TestDataURLAndMediaTypeCaseNormalizationPreserveBytes(t *testing.T) {
	for _, block := range []map[string]any{
		{"type": "input_file", "file_data": " DATA:text/plain;CHARSET=UTF-8;BASE64,QQ== "},
		{"type": "input_file", "file_data": "data:text/plain;charset=UTF-8;base64,QQ==", "file_url": "DATA:text/plain;charset=utf-8;BASE64,QQ=="},
	} {
		decoded, matched, err := decodeOpenAIInlineFileBlock(block)
		if err != nil || !matched || string(decoded.Data) != "A" || strings.Contains(decoded.ContentType, "DATA:") {
			t.Fatalf("data URL normalization changed media or bytes: %v %+v", err, decoded)
		}
	}
	for _, raw := range []string{"data:text/plain;base64junk,QQ==", "data:text/plain;base64;base64,QQ=="} {
		if _, _, err := decodeDataURL(raw, ""); err == nil {
			t.Fatal("invalid encoding flag was silently accepted")
		}
	}
	data, contentType, err := decodeDataURL(`data:text/plain;note="left;base64;right",QQ==`, "")
	if err != nil || string(data) != "QQ==" || !strings.Contains(contentType, `note="left;base64;right"`) {
		t.Fatalf("quoted MIME value changed payload encoding: %v %s %q", err, contentType, data)
	}
	if _, _, err := decodeOpenAIInlineFileBlock(map[string]any{"type": "input_file", "file_data": "data:text/plain;charset=utf-8;base64,QQ==", "url": "data:text/plain;charset=latin1;base64,QQ=="}); err == nil {
		t.Fatal("conflicting MIME parameters disappeared before source validation")
	}
}

func TestNativeBridgePreservesExistingIDsAndDataURLBytes(t *testing.T) {
	for _, native := range []map[string]any{
		{"type": "image", "file_id": "existing-image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "obsolete"}},
		{"type": "document", "file_id": "existing-document"},
		{"type": "document", "source": map[string]any{"type": "base64", "media_type": "text/plain", "data": "data:text/plain;base64,QQ=="}},
		{"type": "image", "source": map[string]any{"type": "url", "url": "data:image/png;base64,QQ=="}},
	} {
		before, _ := json.Marshal(native)
		block, ok := promptcompat.NormalizeAttachmentContent(native)
		if !ok {
			t.Fatalf("attachment disappeared: %s", before)
		}
		uploader := &nativeFileUploader{}
		request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(err)
		}
		if id, _ := native["file_id"].(string); id != "" {
			refs := promptcompat.CollectOpenAIRefFileIDs(request)
			if len(uploader.uploads) != 0 || len(refs) != 1 || refs[0] != id {
				t.Fatal("bridge discarded an uploaded reference and tried stale bytes")
			}
		} else if len(uploader.uploads) != 1 || string(uploader.uploads[0].Data) != "A" {
			t.Fatalf("native data URL was not uploaded once: %+v", uploader.uploads)
		}
		after, _ := json.Marshal(native)
		if string(before) != string(after) {
			t.Fatal("native conversion mutated its source")
		}
		if strings.Contains(promptcompat.RenderPromptText(request["messages"].([]any), false), "QQ==") {
			t.Fatal("native payload entered chat text")
		}
	}
}
