package contextfile

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestReferencedTextAndCallIdentitySurviveExternalizationAndReupload(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
		map[string]any{"role": "system", "content": "SYSTEM_RULE"},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "document", "file_id": "file-original", "source": map[string]any{"type": "text", "data": "DOCUMENT_BODY"}}}},
		map[string]any{"type": "function_call", "id": "fc-source", "call_id": "call-original", "name": "lookup", "arguments": `{"n":9007199254740993}`},
		map[string]any{"type": "function_call_output", "call_id": "call-original", "output": "TOOL_RESULT"},
		map[string]any{"role": "user", "content": "LATEST_TASK"},
	}}
	standard, err := promptcompat.NormalizeOpenAIResponsesRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	uploader := &recordingUploader{}
	split, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, standard)
	if err != nil || !split.CurrentInputFileApplied {
		t.Fatal("externalization failed", err)
	}
	manifest := split.ContextManifest()
	if len(manifest.HistoryMessages) != 4 || len(manifest.LiveMessages) != 3 {
		t.Fatalf("incorrect transport evidence: history=%d live=%d", len(manifest.HistoryMessages), len(manifest.LiveMessages))
	}
	for i, message := range manifest.HistoryMessages {
		if message.NormalizedMessage != manifest.SourceMessages[i].NormalizedMessage || message.NormalizedMessage.Bytes == 0 {
			t.Fatalf("history representation changed for source %d", i)
		}
	}
	for _, want := range []string{"DOCUMENT_BODY", "file-original", "fc-source", "call-original", "9007199254740993", "TOOL_RESULT"} {
		if !strings.Contains(split.HistoryText, want) || !strings.Contains(split.PromptTokenText, want) {
			t.Fatalf("history or usage lost %s", want)
		}
	}
	if strings.Contains(split.HistoryText, "LATEST_TASK") || !strings.Contains(split.FinalPrompt, "LATEST_TASK") {
		t.Fatal("latest task changed transport")
	}
	history, _ := uploader.uploadFor(currentInputFilename)
	if string(history.Data) != split.HistoryText {
		t.Fatal("uploaded history does not match retained history")
	}
	second := &recordingUploader{}
	replayed, err := (Service{DS: second}).ReuploadAppliedCurrentInputFile(context.Background(), &auth.RequestAuth{}, split)
	if err != nil || !reflect.DeepEqual(replayed.ContextManifest().HistoryMessages, manifest.HistoryMessages) || string(second.uploads[0].Data) != split.HistoryText {
		t.Fatal("reupload changed content or evidence", err)
	}
	manifest.HistoryMessages[0].Fields[0].Name = "MUTATED_COPY"
	if replayed.ContextManifest().HistoryMessages[0].Fields[0].Name == "MUTATED_COPY" {
		t.Fatal("manifest exposed mutable request state")
	}
}
