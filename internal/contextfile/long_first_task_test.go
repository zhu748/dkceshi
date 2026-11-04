package contextfile

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"strings"
	"testing"
)

func TestLargeFirstTaskWithInstructionsMovesExactUserTextIntoFile(t *testing.T) {
	task := "  " + strings.Repeat("中文\n", maxInlineFirstTaskChars/2) + "END_OF_TASK\n "
	messages := []any{map[string]any{"role": "system", "content": "SYSTEM RULE"}, map[string]any{"role": "developer", "content": "DEVELOPER RULE"}, map[string]any{"role": "user", "content": task}}
	req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", Messages: messages, ToolChoice: promptcompat.DefaultToolChoicePolicy(), RefFileIDs: []string{"original"}}
	req.FinalPrompt, _ = req.BuildPromptFromMessages(messages)
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || !out.CurrentInputFileApplied {
		t.Fatalf("split failed: %v", err)
	}
	if !strings.Contains(out.HistoryText, task) || strings.Contains(out.FinalPrompt, task) {
		t.Fatal("large task was lost or kept inline")
	}
	for _, rule := range []string{"SYSTEM RULE", "DEVELOPER RULE"} {
		if !strings.Contains(out.FinalPrompt, rule) {
			t.Fatal("live instruction lost")
		}
	}
	if len(out.RefFileIDs) != 2 || out.RefFileIDs[1] != "original" || !strings.Contains(out.PromptTokenText, task) {
		t.Fatal("split lost references or usage context")
	}
	if len(req.Messages) != 3 || req.Messages[2].(map[string]any)["content"] != task {
		t.Fatal("caller messages mutated")
	}
	if out.ArchiveUserInput != task {
		t.Fatal("externalized current input was lost from archive metadata")
	}
}

func TestLargeFirstActiveToolLoopStillStaysTogether(t *testing.T) {
	task := strings.Repeat("task", maxInlineFirstTaskChars)
	messages := []any{map[string]any{"role": "system", "content": "RULE"}, map[string]any{"role": "user", "content": task}, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a", "name": "read", "input": map[string]any{}}}}, map[string]any{"role": "tool", "tool_call_id": "a", "content": "result"}}
	req := promptcompat.StandardRequest{Messages: messages}
	if planCurrentInputContext(req) != nil {
		t.Fatal("active first tool loop was separated from its user task")
	}
}
