package history

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"strings"
	"testing"
)

func TestFileSplitKeepsActiveToolLoopAndImageLive(t *testing.T) {
	messages := []any{
		map[string]any{"role": "system", "content": "OUTPUT JSON"},
		map[string]any{"role": "user", "content": "OLD TASK"},
		map[string]any{"role": "assistant", "content": "OLD ANSWER"},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_image", "file_id": "current_image"}}},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "current_call", "function": map[string]any{"name": "read_file", "arguments": `{"path":"a"}`}}}},
		map[string]any{"role": "tool", "tool_call_id": "current_call", "content": "CURRENT RESULT"},
	}
	req := promptcompat.StandardRequest{Messages: messages, ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: promptcompat.RenderPromptText(messages, false), ToolChoice: promptcompat.DefaultToolChoicePolicy(), RefFileIDs: []string{"current_image"}}
	uploader := &recordingUploader{}
	out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
	if err != nil || !out.CurrentInputFileApplied {
		t.Fatalf("split failed: %v", err)
	}
	for _, want := range []string{"OUTPUT JSON", "current_image", "current_call", "CURRENT RESULT"} {
		if !strings.Contains(out.FinalPrompt, want) {
			t.Fatalf("active context lost %s", want)
		}
	}
	if strings.Contains(out.HistoryText, "CURRENT RESULT") || strings.Contains(out.FinalPrompt, "OLD ANSWER") {
		t.Fatal("wrong history boundary")
	}
	if !strings.Contains(out.PromptTokenText, "OLD ANSWER") || !strings.Contains(out.PromptTokenText, "CURRENT RESULT") {
		t.Fatal("usage misses context")
	}
}
