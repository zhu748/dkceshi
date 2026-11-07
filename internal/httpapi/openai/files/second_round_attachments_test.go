package files

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestNestedInlineFileKeepsCallerFilenameAndOtherLabels(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "file", "filename": "CALLER_FILE.txt", "name": "CALLER_LABEL", "file": map[string]any{"file_data": "data:text/plain;base64,SEVMTE8="}},
	}}}}
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{}, req, &requestAttachmentUploader{}, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CALLER_FILE.txt", "CALLER_LABEL"} {
		if !strings.Contains(std.FinalPrompt, want) {
			t.Errorf("nested upload discarded %s: %s", want, std.FinalPrompt)
		}
	}
}

func TestNestedFileMetadataSurvivesAccountReplay(t *testing.T) {
	source := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "  TASK\r\n"}}, "files": []any{
		map[string]any{"filename": "WRAPPER.txt", "name": "WRAPPER_LABEL", "displayName": "FILE_LABEL", "file": map[string]any{"type": "input_file", "file_data": "data:text/plain;base64,SEVMTE8="}},
	}}
	uploader := &requestAttachmentUploader{}
	working := cloneFileInput(source).(map[string]any)
	if err := PreprocessInlineFiles(context.Background(), &auth.RequestAuth{AccountID: "old"}, working, uploader, "deepseek-v4.1-flash", false); err != nil {
		t.Fatal(err)
	}
	standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, working, "")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &inlineReplaySnapshot{source: source, uploader: uploader, model: "deepseek-v4.1-flash", owner: "old"}
	standard = WithReplay(standard, &auth.RequestAuth{AccountID: "new"}, map[string]any{replaySnapshotKey: snapshot}, func(_ context.Context, req map[string]any) (promptcompat.StandardRequest, error) {
		return promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
	})
	replayed, err := standard.ReplayInput(context.Background())
	if err != nil || uploader.uploads != 2 || len(replayed.RefFileIDs) != 1 || replayed.RefFileIDs[0] == standard.RefFileIDs[0] {
		t.Fatalf("account replay did not rebuild the attachment: %v refs=%v", err, replayed.RefFileIDs)
	}
	for _, text := range []string{replayed.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(replayed.Messages)} {
		for _, want := range []string{"WRAPPER.txt", "WRAPPER_LABEL", "FILE_LABEL", "  TASK\r\n"} {
			if !strings.Contains(text, want) {
				t.Errorf("replay lost %q: %s", want, text)
			}
		}
		if strings.Contains(text, "SEVMTE8=") || strings.Contains(text, standard.RefFileIDs[0]) {
			t.Fatal("replay retained raw binary or the old account file ID")
		}
	}
}
