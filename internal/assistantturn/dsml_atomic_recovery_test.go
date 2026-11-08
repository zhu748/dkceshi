package assistantturn

import (
	"context"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestDSMLRecoveryNeverDeliversAnIncompleteBatch(t *testing.T) {
	valid := `<DSML calls><DSML invoke name="read"><DSML parameter name="path" string="true">/tmp/a</DSML parameter></DSML invoke></DSML calls>`
	for _, fragment := range []string{`<tool_calls>`, `<|EPSE|tool_calls>`, `<tool_calls><invoke`, `<|EPSE|invoke`, `<tool_calls><invoke name="write"`, `<DSML calls`} {
		for _, raw := range []string{valid + fragment, fragment + valid} {
			if calls, ok := toolcall.RecoverDSMLToolCalls(raw); ok || len(calls) > 0 {
				t.Fatalf("partial local recovery: %q", raw)
			}
			opts := BuildOptions{ToolNames: []string{"read", "write"}}
			for _, turn := range []Turn{
				BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts),
				BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts),
			} {
				outcome := FinalizeTurn(turn, FinalizeOptions{})
				if !outcome.ShouldFail || outcome.Error.Code != "invalid_tool_arguments" {
					t.Fatalf("incomplete batch delivered: %q outcome=%+v", raw, outcome)
				}
			}
		}
	}
}

func TestModelRepairCannotDiscardAnIncompleteSiblingAfterDSML(t *testing.T) {
	valid := `<DSML calls><DSML invoke name="read"><DSML parameter name="path" string="true">/tmp/a</DSML parameter></DSML invoke></DSML calls>`
	replacement := `<tool_calls><invoke name="read"><parameter name="path">/tmp/a</parameter></invoke></tool_calls>`
	for _, tail := range []string{`<tool_calls>`, `<|EPSE|tool_calls>`, `<invoke name="write"`, `<|EPSE|invoke`, `<tool_calls><invoke name="write"`} {
		opts := BuildOptions{ToolNames: []string{"read", "write"}, ToolCallRepair: func(context.Context, string) (string, error) { return replacement, nil }}
		turn := BuildTurnFromCollected(sse.CollectResult{Text: valid + tail}, opts)
		if turn.Error == nil {
			t.Fatalf("model repair dropped incomplete sibling %q", tail)
		}
	}
}

func TestModelRepairCanCloseAKnownInvocation(t *testing.T) {
	broken := `<tool_calls><invoke name="read"><parameter name="path">/tmp/a</parameter>`
	opts := BuildOptions{ToolNames: []string{"read"}, ToolCallRepair: func(context.Context, string) (string, error) {
		return broken + `</invoke></tool_calls>`, nil
	}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: broken}, opts)
	if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["path"] != "/tmp/a" {
		t.Fatalf("known source identities stopped being repairable: %+v", turn)
	}
}

func TestDSMLRecoveryRetainsCanonicalSiblingsAndLiteralMarkup(t *testing.T) {
	text := `<DSML calls><DSML invoke name="read"><DSML parameter name="path" string="true">literal &amp; &lt;tool_calls&gt;</DSML parameter></DSML invoke></DSML calls>` +
		`<tool_calls><invoke name="write"><parameter name="content"><![CDATA[<tool_calls><invoke name="literal"]]></parameter></invoke></tool_calls>`
	opts := BuildOptions{ToolNames: []string{"read", "write"}}
	turn := BuildTurnFromCollected(sse.CollectResult{Text: text}, opts)
	if turn.Error != nil || len(turn.ToolCalls) != 2 || turn.ToolCalls[0].Name != "read" || turn.ToolCalls[1].Name != "write" {
		t.Fatalf("complete mixed batch rejected: %+v", turn)
	}
	if turn.ToolCalls[0].Input["path"] != "literal &amp; &lt;tool_calls&gt;" || turn.ToolCalls[1].Input["content"] != `<tool_calls><invoke name="literal"` {
		t.Fatal("recovery rewrote literal argument bytes")
	}
}
