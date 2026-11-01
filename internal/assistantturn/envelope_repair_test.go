package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestUnknownEnvelopeUsesIntegrityCheckedRepair(t *testing.T) {
	raw := `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path"><![CDATA[a.txt]]></|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	for _, changeName := range []bool{false, true} {
		attempts := 0
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: []string{"read"}, ToolCallRepair: func(_ context.Context, prompt string) (string, error) {
			attempts++
			// Repair only shells in the slotted source; preserve the randomized
			// parameter placeholders exactly as the real repair contract requires.
			start := strings.LastIndex(prompt, "<|MISTYPED|broken>")
			if start < 0 {
				t.Fatal("repair did not receive original candidate")
			}
			out := strings.ReplaceAll(prompt[start:], "MISTYPED", "EPSE")
			out = strings.ReplaceAll(out, "|broken>", "|tool_calls>")
			if changeName {
				out = strings.ReplaceAll(out, `name="read"`, `name="write"`)
			}
			return out, nil
		}})
		if attempts != 1 {
			t.Fatalf("repair attempts=%d", attempts)
		}
		if changeName {
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || len(turn.ToolCalls) != 0 {
				t.Fatalf("changed identity accepted: %+v", turn)
			}
		} else if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["path"] != "a.txt" || turn.Text != "" {
			t.Fatalf("repair failed: %+v", turn)
		}
	}
}
