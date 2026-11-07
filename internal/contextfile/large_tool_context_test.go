package contextfile

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestLargeToolResultExternalizesEntireLoopAndKeepsPrefill(t *testing.T) {
	result := "  " + strings.Repeat("工具结果\r\n", 10000) + "RESULT_END\n "
	for _, prefill := range []bool{false, true} {
		messages := []any{
			map[string]any{"role": "system", "content": "SYSTEM_RULE"},
			map[string]any{"role": "user", "content": "OLD_TASK"}, map[string]any{"role": "assistant", "content": "OLD_ANSWER"},
			map[string]any{"role": "user", "content": "ACTIVE_TASK"},
			map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "active_call", "name": "read", "input": map[string]any{"path": "source"}}}},
			map[string]any{"role": "tool", "tool_call_id": "active_call", "content": result},
		}
		if prefill {
			messages = append(messages, map[string]any{"role": "assistant", "prefix": true, "content": "  PREFILL"})
		}
		req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", Messages: messages, ToolChoice: promptcompat.DefaultToolChoicePolicy()}
		req.FinalPrompt, _ = req.BuildPromptFromMessages(messages)
		uploader := &recordingUploader{}
		out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
		if err != nil || !out.CurrentInputFileApplied || strings.Contains(out.FinalPrompt, result) || !strings.Contains(out.HistoryText, result) || !strings.Contains(out.FinalPrompt, "SYSTEM_RULE") {
			t.Fatalf("large active context not preserved: %v", err)
		}
		if strings.Index(out.HistoryText, "ACTIVE_TASK") > strings.Index(out.HistoryText, `call_id="active_call"`) || strings.Index(out.HistoryText, `call_id="active_call"`) > strings.Index(out.HistoryText, "RESULT_END") {
			t.Fatal("externalization reordered task/call/result")
		}
		if prefill && (!strings.HasSuffix(out.FinalPrompt, "[Assistant]:  PREFILL") || strings.Contains(out.HistoryText, "PREFILL")) {
			t.Fatal("prefill no longer applies to the live generation")
		}
		if len(out.ContextManifest().SourceMessages) != len(messages) || !strings.Contains(out.PromptTokenText, result) {
			t.Fatal("archive or usage context lost original messages")
		}
	}
}
