package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

func TestDisabledToolsNeverInvokeFormatRepair(t *testing.T) {
	for _, policy := range []promptcompat.ToolChoicePolicy{{Mode: promptcompat.ToolChoiceNone}, {RestrictAllowed: true}} {
		invoked := false
		opts := BuildOptions{ToolChoice: policy, ToolCallRepair: func(context.Context, string) (string, error) {
			invoked = true
			return "", nil
		}}
		raw := `<|EPSE|call name="read"><|EPSE|parameter name="path"><![CDATA[a]]></|EPSE|parameter></|EPSE|call>`
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts)
		streamTurn := BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts)
		if invoked || turn.Error == nil || streamTurn.Error == nil {
			t.Fatalf("disabled tool output repaired or accepted: invoked=%v collected=%v stream=%v", invoked, turn.Error, streamTurn.Error)
		}
	}
}
