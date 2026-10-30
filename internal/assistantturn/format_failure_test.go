package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/sse"
)

func TestUnrepairableClientCallFailsBeforeDelivery(t *testing.T) {
	good := `<|EPSE|tool_calls><|EPSE|invoke name="read"></|EPSE|invoke></|EPSE|tool_calls>`
	bad := `<|EPSE|call name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter></|EPSE|call>`
	for _, raw := range []string{bad, good + bad, `<tool_calls><invoke name="write"><parameter name="path">a.txt</parameter><parameter name="optional">lost</invoke></tool_calls>`} {
		for _, reply := range []string{"", "NO TOOL", good} {
			opts := BuildOptions{ToolNames: []string{"read", "write"}}
			if reply != "" {
				opts.ToolCallRepair = func(context.Context, string) (string, error) { return reply, nil }
			}
			for _, turn := range []Turn{
				BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
				BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts),
			} {
				outcome := FinalizeTurn(turn, FinalizeOptions{})
				if !outcome.ShouldFail || outcome.Error.Code != "invalid_tool_arguments" {
					t.Fatalf("malformed call was accepted: %#v", turn)
				}
				if ShouldRetryEmptyOutput(turn, 0, 3) {
					t.Fatal("tool format failure must not trigger the empty-output retry loop")
				}
			}
		}
	}
}

func TestFormatGuardPreservesProseExamplesAndOrphanShells(t *testing.T) {
	for _, raw := range []string{
		"normal answer",
		"```xml\n<|EPSE|call name=\"write\">example</|EPSE|call>\n```",
		"`<|EPSE|call name=\"write\">example</|EPSE|call>`",
		"answer </｜DSML｜calls> done",
	} {
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"write"}})
		if turn.Error != nil || turn.Text == "" || len(turn.ToolCalls) != 0 {
			t.Fatalf("ordinary text was classified as a broken invocation: %#v", turn)
		}
	}
	good := `<|EPSE|tool_calls><|EPSE|invoke name="write"></|EPSE|invoke></|EPSE|tool_calls>`
	turn := BuildTurnFromCollected(sse.CollectResult{Text: good + `</｜DSML｜calls>`}, BuildOptions{ToolNames: []string{"write"}})
	if turn.Error != nil || len(turn.ToolCalls) != 1 {
		t.Fatalf("orphan DSML shell invalidated a complete call: %#v", turn)
	}
}
