package contextfile

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestLargeRequestAttachmentContextSurvivesExternalization(t *testing.T) {
	metadata := strings.Repeat("附件说明", 10000) + "CONTEXT_END"
	req, err := promptcompat.NormalizeOpenAIChatRequest(nil, map[string]any{
		"model": "deepseek-v4.1-flash", "messages": []any{
			map[string]any{"role": "system", "content": "SYSTEM"}, map[string]any{"role": "user", "content": "ACTIVE_TASK"},
			map[string]any{"role": "assistant", "prefix": true, "content": "  PREFILL"},
		}, "files": []any{map[string]any{"file_id": "original-file", "filename": "ORIGINAL.txt", "description": metadata}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || !out.CurrentInputFileApplied || !strings.Contains(out.HistoryText, metadata) || !strings.Contains(out.HistoryText, "ORIGINAL.txt") || !strings.Contains(out.HistoryText, "ACTIVE_TASK") {
		t.Fatalf("attachment context lost during externalization: %v", err)
	}
	if !strings.HasSuffix(out.FinalPrompt, "[Assistant]:  PREFILL") || len(out.RefFileIDs) != 2 || out.RefFileIDs[1] != "original-file" {
		t.Fatal("attachment context displaced prefill or references")
	}
	if out.ContextManifest().InputFields[len(out.ContextManifest().InputFields)-1].Name != "files" {
		t.Fatal("source attachment fingerprint lost during externalization")
	}
}
