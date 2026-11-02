package completionruntime

import (
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/sse"
)

func TestRecoveredToolTextStopsAtOriginalBoundary(t *testing.T) {
	canonical := `<tool_calls><invoke name="read"><parameter name="path"><![CDATA[a.txt]]></parameter></invoke></tool_calls>`
	dsml := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"><｜｜DSML｜｜ parameter name="path" string="true">a.txt</｜｜DSML｜｜ parameter></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
	for _, call := range []string{canonical, dsml} {
		for _, prefix := range []string{"I will read it. ", "Example: `" + canonical + "`\n", "~~~xml\n" + dsml + "\n~~~\n", `"<tool_calls>" `, `<![CDATA[<tool_calls>]]> `} {
			raw := prefix + call + " DO NOT RELEASE THIS FAKE RESULT"
			turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{})
			if turn.Error != nil || len(turn.ToolCalls) != 1 {
				t.Fatalf("call not recovered: %+v", turn)
			}
			got := ValidatedToolText(turn, nil, "", false)
			if got != prefix {
				t.Fatalf("got=%q want=%q", got, prefix)
			}
		}
	}
}
