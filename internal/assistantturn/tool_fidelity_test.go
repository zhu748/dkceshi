package assistantturn

import (
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestRequiredToolChoicePreservesUpstreamFailure(t *testing.T) {
	for _, policy := range []promptcompat.ToolChoicePolicy{
		{Mode: promptcompat.ToolChoiceRequired},
		{Mode: promptcompat.ToolChoiceForced, ForcedName: "lookup"},
	} {
		for _, tc := range []struct {
			turn Turn
			code string
		}{
			{Turn{}, "upstream_unavailable"},
			{Turn{Thinking: "reasoning only"}, "upstream_empty_output"},
			{Turn{ContentFilter: true}, "content_filter"},
			{Turn{UpstreamError: "upstream failed"}, "upstream_error"},
			{Turn{Text: "answer without a tool"}, "tool_choice_violation"},
		} {
			if err := ValidateTurn(tc.turn, policy); err == nil || err.Code != tc.code {
				t.Fatalf("policy=%s: expected %s, got %#v", policy.Mode, tc.code, err)
			}
		}
	}
}

func TestBuildTurnPreservesCallerMarkerInToolArguments(t *testing.T) {
	const marker = "Q7ZK3M"
	const literal = `C:/Q7ZK3M/file.txt <|Q7ZK3M|invoke> 中文`
	markup := `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="content"><![CDATA[` + literal + `]]></|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	opts := BuildOptions{ToolMarker: marker, ToolNames: []string{"write"}}
	// Stream finalization receives already-normalized text; it must not change
	// literal markers during the independent final parse either.
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: markup}, opts),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: toolcall.NormalizeMarkerText(markup, marker)}, opts),
	} {
		if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["content"] != literal {
			t.Fatalf("literal tool argument changed: %#v", turn)
		}
	}
}
