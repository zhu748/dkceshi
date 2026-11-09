package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestTextDocumentOwnAttachmentsAreUploadedAndReferenced(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		for _, onSource := range []bool{false, true} {
			source := map[string]any{"type": "text", "data": "DOCUMENT_BODY"}
			document := map[string]any{"type": "document", "source": source}
			container := document
			if onSource {
				container = source
			}
			container[key] = []any{map[string]any{"type": "input_file", "filename": "source.txt", "file_data": "data:text/plain;base64,QQ==", "description": "NESTED_ATTACHMENT"}}
			request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{document}}}}
			uploader := &nativeFileUploader{}
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil {
				t.Fatal(err)
			}
			standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
			if err != nil || len(uploader.uploads) != 1 || len(standard.RefFileIDs) != 1 || strings.Contains(standard.FinalPrompt, "QQ==") || !strings.Contains(standard.FinalPrompt, "DOCUMENT_BODY") || !strings.Contains(standard.FinalPrompt, "NESTED_ATTACHMENT") {
				t.Fatalf("text document attachment omitted: err=%v uploads=%d refs=%v", err, len(uploader.uploads), standard.RefFileIDs)
			}
		}
	}
}

func TestUploadedIDConflictFailsBeforeUploadAndDoesNotExposeValues(t *testing.T) {
	block := map[string]any{"type": "input_file", "file_id": "PRIVATE_FILE_A", "file": map[string]any{"file_id": "PRIVATE_FILE_B"}}
	uploader := &nativeFileUploader{}
	request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
	err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false)
	if status, _ := InlineFileErrorDetails(err); err == nil || status != 400 || len(uploader.uploads) > 0 {
		t.Fatal("conflicting references were not rejected", err)
	}
	if strings.Contains(err.Error(), "PRIVATE_FILE_") {
		t.Fatal("reference validation disclosed caller IDs")
	}
}

func TestExistingReferenceDoesNotReprocessStaleNestedBytes(t *testing.T) {
	for _, stale := range []string{"!!invalid!!", "data:text/plain;base64,QQ=="} {
		for _, ownAttachment := range []bool{false, true} {
			block := map[string]any{"type": "input_file", "file_id": "file-existing", "file": map[string]any{"file_data": stale}}
			if ownAttachment {
				block["attachments"] = []any{map[string]any{"type": "input_file", "filename": "new.txt", "file_data": "data:text/plain;base64,Qg=="}}
			}
			request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
			uploader := &nativeFileUploader{}
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil {
				t.Fatal("stale locator bytes were interpreted as a new attachment", err)
			}
			want := 0
			if ownAttachment {
				want = 1
			}
			standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
			if err != nil || len(uploader.uploads) != want || len(standard.RefFileIDs) != want+1 {
				t.Fatalf("authoritative reference or its own attachment changed: uploads=%d refs=%v err=%v", len(uploader.uploads), standard.RefFileIDs, err)
			}
		}
	}
}
