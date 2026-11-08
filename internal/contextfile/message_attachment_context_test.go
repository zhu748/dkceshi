package contextfile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestMessageAttachmentDescriptionSurvivesContextExternalization(t *testing.T) {
	description := "ATTACHMENT_START" + strings.Repeat("附件说明\n", 12000) + "ATTACHMENT_END"
	encoded, _ := json.Marshal(description)
	quotedDescription := string(encoded)
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "system", "content": "SYSTEM_RULE"},
		map[string]any{"role": "user", "content": "TASK", "attachments": []any{map[string]any{"type": "input_file", "file_id": "source-file", "description": description}}},
	}}
	standard, err := promptcompat.NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: &recordingUploader{}}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, standard)
	if err != nil || !out.CurrentInputFileApplied || strings.Count(out.HistoryText, quotedDescription) != 1 || strings.Contains(out.FinalPrompt, quotedDescription) {
		t.Fatalf("externalization lost/duplicated message attachments: %v", err)
	}
	if !strings.Contains(out.HistoryText, "source-file") || !strings.Contains(out.PromptTokenText, quotedDescription) || len(out.RefFileIDs) != 2 {
		t.Fatal("transport or usage lost source context")
	}
	found := false
	for _, field := range out.ContextManifest().InputFields {
		found = found || field.Path == "/messages/1/attachments"
	}
	if !found {
		t.Fatal("history splitting discarded attachment source evidence")
	}
}
