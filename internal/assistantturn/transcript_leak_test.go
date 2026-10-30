package assistantturn

import (
	"testing"

	"ds2api/internal/sse"
)

func TestTurnCutsTranscriptBoundaryBeforeInventedCalls(t *testing.T) {
	input := "done\n[EndMessage:3237101fbf162c60]\n[Assistant]:<tool_calls><invoke name=\"read\"></invoke></tool_calls>"
	opts := BuildOptions{ToolNames: []string{"read"}}
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: input}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: input, VisibleText: input}, opts),
	} {
		if turn.Text != "done\n" || len(turn.ToolCalls) != 0 || turn.Error != nil {
			t.Fatalf("transcript continuation leaked: text=%q calls=%#v err=%v", turn.Text, turn.ToolCalls, turn.Error)
		}
	}
}

func TestTurnStripsOrphanClosesWithoutChangingCodeExamples(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"done</|EPSE|tool_calls>", "done"},
		{"`</|EPSE|tool_calls>`", "`</|EPSE|tool_calls>`"},
		{"```xml\n</|EPSE|tool_calls>\n```", "```xml\n</|EPSE|tool_calls>\n```"},
	} {
		for _, turn := range []Turn{
			BuildTurnFromCollected(sse.CollectResult{Text: tc.input}, BuildOptions{}),
			BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: tc.input, VisibleText: tc.input}, BuildOptions{}),
		} {
			if turn.Text != tc.want {
				t.Fatalf("input=%q got=%q want=%q", tc.input, turn.Text, tc.want)
			}
		}
	}
}
