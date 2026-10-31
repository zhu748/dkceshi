package assistantturn

import (
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestAmbiguousJSONArgumentsCannotExecutePartially(t *testing.T) {
	const bad = `<|EPSE|tool_calls><|EPSE|invoke name="read">{"input":{},"parameters":{"path":"lost.txt"}}</|EPSE|invoke></|EPSE|tool_calls>`
	const good = `<|EPSE|tool_calls><|EPSE|invoke name="read"><|EPSE|parameter name="path">ok.txt</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, raw := range []string{bad, good + bad, strings.ReplaceAll(bad, "|EPSE|", ""), strings.ReplaceAll(good+bad, "|EPSE|", "")} {
		opts := BuildOptions{ToolNames: []string{"read"}}
		for _, turn := range []Turn{
			BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
			BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, opts),
		} {
			if outcome := FinalizeTurn(turn, FinalizeOptions{}); !outcome.ShouldFail || outcome.Error.Code != "invalid_tool_arguments" {
				t.Fatalf("ambiguous arguments escaped validation: %+v", turn)
			}
		}
	}
}
