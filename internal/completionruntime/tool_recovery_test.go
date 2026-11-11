package completionruntime

import (
	"ds2api/internal/assistantturn"
	"ds2api/internal/toolcall"
	"testing"
)

func TestToolRecoveryDeliveryBoundary(t *testing.T) {
	for _, code := range []string{"invalid_tool_arguments", "tool_choice_violation", "upstream_unavailable", "content_filter"} {
		failure := &assistantturn.OutputError{Code: code}
		for _, text := range []bool{false, true} {
			for _, tools := range []bool{false, true} {
				want := !tools && (IsToolOutputError(failure) || !text && code == "upstream_unavailable")
				if got := CanDeferToolAwareOutputError(failure, true, text, tools); got != want {
					t.Fatalf("code=%s text=%v tools=%v got=%v", code, text, tools, got)
				}
				if CanDeferToolAwareOutputError(failure, false, text, tools) {
					t.Fatal("disabled/exhausted retry allowed")
				}
			}
		}
	}
	var recovery ToolRecovery
	recovery.Start("invalid_tool_arguments", true)
	recovery.Start("tool_choice_violation", true)
	if !recovery.Hold || recovery.Text(assistantturn.Turn{Text: "duplicate", ToolCalls: []toolcall.ParsedToolCall{{Name: "read"}}}, nil, "", false) != "" {
		t.Fatal("replacement preamble escaped")
	}
	if got := recovery.Text(assistantturn.Turn{Text: "Actual final answer"}, nil, "", false); got != "Actual final answer" {
		t.Fatalf("text-only recovery lost: %q", got)
	}
}
