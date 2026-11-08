package files

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestSystemAliasesShareOneUploadTraversal(t *testing.T) {
	var attachments []any
	for i := 0; i < 8; i++ {
		attachments = append(attachments, map[string]any{"type": "input_file", "filename": fmt.Sprintf("file-%d.txt", i), "file_data": "QUJDRA=="})
	}
	system := map[string]any{"parts": []any{map[string]any{"text": "rules", "attachments": attachments}}}
	req := map[string]any{"systemInstruction": system, "system_instruction": cloneFileInput(system)}
	uploader := &requestAttachmentUploader{}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	if uploader.uploads != 8 || len(promptcompat.CollectOpenAIRefFileIDs(req)) != 8 {
		t.Fatalf("alias duplicated uploads: %d", uploader.uploads)
	}
	if !reflect.DeepEqual(req["systemInstruction"], req["system_instruction"]) {
		t.Fatal("preprocessing created conflicting aliases")
	}
}

func TestConflictingSystemAliasesFailBeforeUploading(t *testing.T) {
	uploader := &requestAttachmentUploader{}
	req := map[string]any{"systemInstruction": map[string]any{"text": "rules", "files": []any{map[string]any{"type": "input_file", "file_data": "QUJDRA=="}}}, "system_instruction": map[string]any{"text": "other rules"}}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, uploader, "deepseek-v4.1-flash", false); err == nil || uploader.uploads != 0 {
		t.Fatal("conflicting aliases reached upload")
	}
}

func TestSystemAttachmentBytesRemainAvailableForAccountReplay(t *testing.T) {
	uploader := &requestAttachmentUploader{}
	source := map[string]any{"system": []any{map[string]any{"type": "text", "text": "rules", "files": []any{map[string]any{"type": "input_file", "file_data": "QUJDRA=="}}}}}
	original := cloneFileInput(source)
	snapshot := &inlineReplaySnapshot{source: source, uploader: uploader, model: "deepseek-v4.1-flash", owner: "old"}
	standard := WithReplay(promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash"}, &auth.RequestAuth{}, map[string]any{replaySnapshotKey: snapshot}, func(_ context.Context, req map[string]any) (promptcompat.StandardRequest, error) {
		return promptcompat.StandardRequest{Messages: []any{map[string]any{"role": "system", "content": req["system"]}}, RefFileIDs: promptcompat.CollectOpenAIRefFileIDs(req)}, nil
	})
	for i := 0; i < 2; i++ {
		out, err := standard.ReplayInput(context.Background())
		if err != nil || len(out.RefFileIDs) != 1 {
			t.Fatalf("replay lost system attachment: %v", err)
		}
	}
	if uploader.uploads != 2 || !reflect.DeepEqual(source, original) {
		t.Fatal("replay consumed source bytes")
	}
}
