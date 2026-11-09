package files

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type nestedAttachmentUploader struct{ uploads []dsclient.UploadFileRequest }

func (u *nestedAttachmentUploader) UploadFile(_ context.Context, _ *auth.RequestAuth, r dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	u.uploads = append(u.uploads, r)
	return &dsclient.UploadFileResult{ID: fmt.Sprintf("file-%s", r.Filename)}, nil
}

func TestNewUploadRetainsOwnAttachmentsAndReferenceSet(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		for _, existing := range []bool{false, true} {
			child := map[string]any{"type": "input_file", "filename": "child.txt", "file_data": "data:text/plain;base64,Y2hpbGQ=", "description": "CHILD_CONTEXT"}
			if existing {
				delete(child, "file_data")
				child["file_id"] = "file-child.txt"
			}
			parent := map[string]any{"type": "input_file", "filename": "parent.txt", "file_data": "data:text/plain;base64,cGFyZW50", key: []any{child}}
			source, _ := json.Marshal(parent)
			request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{parent}}}}
			uploader := &nestedAttachmentUploader{}
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", true); err != nil {
				t.Fatal(err)
			}
			want := 2
			if existing {
				want = 1
			}
			if len(uploader.uploads) != want {
				t.Fatalf("%s existing=%v uploads=%d", key, existing, len(uploader.uploads))
			}
			refs := promptcompat.CollectOpenAIRefFileIDs(request)
			if !reflect.DeepEqual(refs, []string{"file-parent.txt", "file-child.txt"}) {
				t.Fatalf("references changed: %v", refs)
			}
			standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
				if strings.Count(text, "CHILD_CONTEXT") != 1 || !strings.Contains(text, "file-child.txt") || strings.Contains(text, "Y2hpbGQ=") || strings.Contains(text, "omitted_binary_payload") {
					t.Fatalf("nested upload lost text or leaked bytes: %s", text)
				}
			}
			after, _ := json.Marshal(parent)
			if string(after) != string(source) {
				t.Fatal("caller-owned attachment changed")
			}
			// A second pass must not re-upload either file or duplicate its context.
			if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", true); err != nil || len(uploader.uploads) != want {
				t.Fatal("upload traversal was not idempotent", err)
			}
		}
	}
}

func TestNestedUploadFailureDoesNotCommitPartialInput(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_file", "filename": "parent.txt", "file_data": "data:text/plain;base64,cGFyZW50", key: []any{map[string]any{"type": "input_file", "file_data": "!!invalid!!"}}}}}}}
		before, _ := json.Marshal(request)
		err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, &nestedAttachmentUploader{}, "deepseek-v4.1-flash", false)
		after, _ := json.Marshal(request)
		status, _ := InlineFileErrorDetails(err)
		if err == nil || status != 400 || string(before) != string(after) {
			t.Fatal("invalid nested input committed partial state", err)
		}
	}
}

func TestNewUploadTraversesTransportContainerAttachmentBags(t *testing.T) {
	for _, container := range []string{"file", "source", "inlineData", "inline_data"} {
		child := map[string]any{"type": "input_file", "filename": "child.txt", "file_data": "data:text/plain;base64,Y2hpbGQ=", "description": "CHILD_CONTEXT"}
		source := map[string]any{"data": "cGFyZW50", "media_type": "text/plain", "attachments": []any{child}}
		parent := map[string]any{"filename": "parent.txt", container: source}
		switch container {
		case "source":
			parent["type"] = "document"
			source["type"] = "base64"
		case "file":
			parent["type"] = "input_file"
			source["file_data"] = "data:text/plain;base64,cGFyZW50"
			delete(source, "data")
		}
		request := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{parent}}}}
		uploader := &nestedAttachmentUploader{}
		if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, request, uploader, "deepseek-v4.1-flash", false); err != nil {
			t.Fatal(container, err)
		}
		history := promptcompat.BuildOpenAIHistoryTranscript(request["messages"].([]any))
		if len(uploader.uploads) != 2 || len(promptcompat.CollectOpenAIRefFileIDs(request)) != 2 || strings.Count(history, "CHILD_CONTEXT") != 1 || strings.Contains(history, "Y2hpbGQ=") {
			t.Fatalf("%s attachment lost or duplicated: %s", container, history)
		}
	}
}

func TestReferenceCoverageRejectsLostExistingOrUploadedIDs(t *testing.T) {
	for _, state := range []*inlineUploadState{
		{expectedRefs: []string{"PRIVATE_CHILD"}},
		{pendingFiles: []*dsclient.UploadFileResult{{ID: "PRIVATE_CHILD"}}},
	} {
		err := state.checkReferenceCoverage([]string{"parent"})
		if err == nil || strings.Contains(err.Error(), "PRIVATE_CHILD") {
			t.Fatal("lost reference was accepted or disclosed", err)
		}
	}
}
