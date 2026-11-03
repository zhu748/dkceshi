package contextfile

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestSplitFirstTaskPreservesInputAndEstablishedPlacement(t *testing.T) {
	task := strings.Repeat("long task body\n", 500)
	for _, withSystem := range []bool{false, true} {
		t.Run(fmt.Sprint(withSystem), func(t *testing.T) {
			messages := []any{}
			if withSystem {
				messages = append(messages, map[string]any{"role": "system", "content": "CURRENT INSTRUCTION"})
			}
			messages = append(messages, map[string]any{"role": "user", "content": task})
			req := promptcompat.StandardRequest{Messages: messages, ResolvedModel: "deepseek-v4.1-flash", ToolChoice: promptcompat.DefaultToolChoicePolicy()}
			req.FinalPrompt, _ = req.BuildPromptFromMessages(messages)
			uploader := &recordingUploader{}
			out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
			if err != nil || !out.CurrentInputFileApplied {
				t.Fatalf("split failed: %v", err)
			}
			if strings.Contains(out.FinalPrompt, task) != withSystem || strings.Contains(out.HistoryText, task) == withSystem {
				t.Fatal("first task placement or content changed")
			}
			if withSystem && !strings.Contains(out.FinalPrompt, "CURRENT INSTRUCTION") {
				t.Fatal("live instructions lost")
			}
			if len(uploader.uploads) != 1 {
				t.Fatal("unexpected upload count")
			}
		})
	}
}

func TestSplitRespectsThresholdAndCombinedFileBudget(t *testing.T) {
	for _, tc := range []struct {
		name  string
		old   string
		refs  int
		tools bool
		split bool
	}{
		{"tiny history", "short", 0, false, true},
		{"long history", strings.Repeat("old context ", 500), 0, false, true},
		{"49 refs with tools", strings.Repeat("old context ", 500), 49, true, false},
		{"48 refs with tools", strings.Repeat("old context ", 500), 48, true, true},
		{"50 refs without tools", strings.Repeat("old context ", 500), 50, false, false},
		{"49 refs without tools", strings.Repeat("old context ", 500), 49, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []any{map[string]any{"role": "user", "content": tc.old}, map[string]any{"role": "assistant", "content": "done"}, map[string]any{"role": "user", "content": "current task"}}
			req := promptcompat.StandardRequest{Messages: messages, ResolvedModel: "deepseek-v4.1-flash", ToolChoice: promptcompat.DefaultToolChoicePolicy()}
			if tc.tools {
				req.ToolsRaw = []any{map[string]any{"name": "read_file", "parameters": map[string]any{"type": "object"}}}
			}
			for i := 0; i < tc.refs; i++ {
				req.RefFileIDs = append(req.RefFileIDs, fmt.Sprintf("client_%d", i))
			}
			req.FinalPrompt, _ = req.BuildPromptFromMessages(messages)
			uploader := &recordingUploader{}
			out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
			if tc.split && (err != nil || !out.CurrentInputFileApplied) {
				t.Fatalf("split=%v error=%v", out.CurrentInputFileApplied, err)
			}
			if !tc.split {
				status, _ := MapError(err)
				if err == nil || status != 400 {
					t.Fatalf("expected explicit file limit error, got %v", err)
				}
			}
			if len(out.RefFileIDs) > promptcompat.MaxReferenceFiles {
				t.Fatal("generated files bypassed limit")
			}
			if !tc.split && (len(uploader.uploads) != 0 || out.FinalPrompt != req.FinalPrompt) {
				t.Fatal("skipped split uploaded or changed input")
			}
		})
	}
}

func TestSplitKeepsFirstActiveToolLoopLive(t *testing.T) {
	messages := []any{map[string]any{"role": "user", "content": strings.Repeat("task ", 1000)}, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a", "function": map[string]any{"name": "read_file", "arguments": "{}"}}}}, map[string]any{"role": "tool", "tool_call_id": "a", "content": "result"}}
	req := promptcompat.StandardRequest{Messages: messages, ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: promptcompat.RenderPromptText(messages, false)}
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || out.CurrentInputFileApplied || len(uploader.uploads) != 0 {
		t.Fatal("active first tool loop must remain live")
	}
}

func TestSplitPreservesProtocolTextCatalogBoundary(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		req := markerStandardRequest(markerUnderTest)
		req.ToolsRaw = nil
		req.SystemTextToolsDisabled = disabled
		req.Messages = append([]any{map[string]any{"role": "system", "content": "Available tools: read_file"}}, req.Messages...)
		req.FinalPrompt, _ = req.BuildPromptFromMessages(req.Messages)
		out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: &recordingUploader{}}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
		if err != nil || !out.CurrentInputFileApplied {
			t.Fatalf("split failed: %v", err)
		}
		if strings.Contains(out.FinalPrompt, "工具调用格式规范") == disabled {
			t.Fatalf("text-catalog policy changed after split: disabled=%v", disabled)
		}
	}
}
