package assistantturn

import (
	"io"
	"net/http"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestCollectedReadErrorCannotBeClearedByParsedTools(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{
		Text:      `<tool_calls><invoke name="run"><parameter name="cmd">ls</parameter></invoke></tool_calls>`,
		ReadError: io.ErrUnexpectedEOF,
	}, BuildOptions{ToolNames: []string{"run"}})
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("expected a parsed call to exercise finalization: %+v", turn)
	}
	outcome := FinalizeTurn(turn, FinalizeOptions{})
	if !outcome.ShouldFail || outcome.Error.Status != http.StatusBadGateway || outcome.Error.Code != "upstream_read_error" {
		t.Fatalf("parsed tools must not hide a failed read: %+v", outcome)
	}
}

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
