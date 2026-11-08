package contextfile

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestLargeMessageExtensionsMoveWithTheirTaskIntoHistory(t *testing.T) {
	extension := strings.Repeat("上下文", 16000) + "EXTENSION_END"
	messages := []any{
		map[string]any{"role": "system", "content": "SYSTEM_RULE"},
		map[string]any{"role": "user", "content": "ACTIVE_TASK", "custom_context": extension},
		map[string]any{"role": "assistant", "prefix": true, "content": "PREFIX", "metadata": "PREFILL_CONTEXT"},
	}
	req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", Messages: messages, ToolChoice: promptcompat.DefaultToolChoicePolicy()}
	req.FinalPrompt, _ = req.BuildPromptFromMessages(messages)
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || !out.CurrentInputFileApplied {
		t.Fatal("large extension was not externalized", err)
	}
	if strings.Contains(out.FinalPrompt, extension) || strings.Count(out.HistoryText, extension) != 1 || !strings.Contains(out.HistoryText, "ACTIVE_TASK") || !strings.Contains(out.PromptTokenText, extension) {
		t.Fatal("externalization lost or duplicated context")
	}
	if !strings.HasSuffix(out.FinalPrompt, "[Assistant]:PREFIX") || !strings.Contains(out.FinalPrompt, "PREFILL_CONTEXT") || strings.Contains(out.HistoryText, "PREFILL_CONTEXT") {
		t.Fatal("prefill and its context were split")
	}
}

func TestLargeToolExtensionsMoveWithLinkedTaskAndUploadedHistory(t *testing.T) {
	extension := strings.Repeat("TOOL_CONTEXT ", 4000) + "TOOL_CONTEXT_END"
	req, err := promptcompat.NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "system", "content": "SYSTEM_RULE"},
		map[string]any{"role": "user", "content": "ACTIVE_TASK"},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"name": "read", "input": map[string]any{"path": "file.go"}, "metadata": extension}}},
		map[string]any{"role": "tool", "name": "read", "content": "TOOL_RESULT"},
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	id := req.Messages[3].(map[string]any)["tool_call_id"].(string)
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || !out.CurrentInputFileApplied {
		t.Fatal("large tool metadata did not externalize its loop", err)
	}
	upload, ok := uploader.uploadFor(currentInputFilename)
	if !ok || string(upload.Data) != out.HistoryText {
		t.Fatal("uploaded history differs from recorded context")
	}
	for _, want := range []string{extension, "ACTIVE_TASK", "TOOL_RESULT", `call_id="` + id + `"`, `"tool_call_id":"` + id + `"`} {
		if strings.Count(out.HistoryText, want) != 1 || !strings.Contains(out.PromptTokenText, want) {
			t.Fatalf("history or usage lost linked context %q", want[:min(80, len(want))])
		}
	}
	if strings.Contains(out.FinalPrompt, "TOOL_CONTEXT_END") || strings.Contains(out.FinalPrompt, "TOOL_RESULT") {
		t.Fatal("tool loop was duplicated in live prompt")
	}
}
