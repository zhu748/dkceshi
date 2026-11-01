package assistantturn

import (
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestUnfinishedToolProtocolCannotBecomeAnswer(t *testing.T) {
	for _, raw := range []string{
		`<|EPSE|tool_calls>`, `answer <|EPSE|invoke name="run"`, `answer <|EPSE|tool_ca`,
		`answer <｜epse｜call name="run">secret</｜epse｜call>`, `answer <invoke name="run"`,
	} {
		for _, names := range [][]string{nil, {"run"}} {
			for _, turn := range []Turn{
				BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolNames: names}),
				BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw}, BuildOptions{ToolNames: names}),
			} {
				if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || len(turn.ToolCalls) != 0 {
					t.Fatalf("protocol became a successful answer: raw=%q turn=%+v", raw, turn)
				}
			}
		}
	}
}

func TestReasoningToolMarkupHiddenButRawPreserved(t *testing.T) {
	raw := `plan <|EPSE|invoke name="run"><|EPSE|parameter name="cmd">secret</|EPSE|parameter></|EPSE|invoke> done`
	a := StreamAccumulator{ThinkingEnabled: true}
	var visible strings.Builder
	for _, ch := range raw {
		out := a.Apply(sse.LineResult{Parts: []sse.ContentPart{{Type: "thinking", Text: string(ch)}}})
		for _, part := range out.Parts {
			visible.WriteString(part.VisibleText)
		}
	}
	for _, part := range a.Apply(sse.LineResult{Flush: true}).Parts {
		visible.WriteString(part.VisibleText)
	}
	if a.RawThinking.String() != raw || visible.String() != "plan  done" {
		t.Fatalf("raw=%q visible=%q", a.RawThinking.String(), visible.String())
	}
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: "answer", Thinking: raw}, BuildOptions{}),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: "answer", VisibleText: "answer", RawThinking: raw, VisibleThinking: raw}, BuildOptions{}),
	} {
		if turn.Error != nil || turn.Thinking != "plan  done" || len(turn.ToolCalls) != 0 {
			t.Fatalf("reasoning escaped or became a body call: %+v", turn)
		}
	}
}

func TestTruncatedSiblingCannotDisappearBesideValidCall(t *testing.T) {
	good := `<tool_calls><invoke name="run"></invoke></tool_calls>`
	for _, bad := range []string{`<|EPSE|tool_ca`, `<|EPSE|tool_calls>`, `<invoke name="run"`} {
		for _, raw := range []string{good + bad} {
			turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{})
			if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" {
				t.Fatalf("unfinished sibling silently lost: raw=%q turn=%+v", raw, turn)
			}
		}
	}
}

func TestToolProtocolExamplesAndOrphanClosesRemainSafe(t *testing.T) {
	for _, raw := range []string{
		"`<|EPSE|tool_calls>`", "```xml\n<|EPSE|tool_calls>\n```", `"<|EPSE|tool_calls>"`,
		`<![CDATA[<|EPSE|tool_calls>]]>`, `<!-- <|EPSE|tool_calls> -->`, "1 < 2", `answer </invoke>`,
	} {
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{})
		if turn.Error != nil || len(turn.ToolCalls) != 0 {
			t.Fatalf("example/orphan rejected: raw=%q error=%+v calls=%+v", raw, turn.Error, turn.ToolCalls)
		}
	}
}

func TestContentFilterCannotReleaseRawToolProtocol(t *testing.T) {
	raw := `answer <|EPSE|tool_calls>secret`
	for _, turn := range []Turn{
		BuildTurnFromCollected(sse.CollectResult{Text: raw, Thinking: raw, ContentFilter: true}, BuildOptions{}),
		BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw, VisibleText: raw, RawThinking: raw, VisibleThinking: raw, ContentFilter: true}, BuildOptions{}),
	} {
		if strings.Contains(turn.Text+turn.Thinking, "secret") || strings.Contains(turn.Text+turn.Thinking, "EPSE") || turn.RawText != raw {
			t.Fatalf("filtered turn released a raw tool fragment or lost diagnostics: %+v", turn)
		}
	}
}

func TestReasoningGuardKeepsContinuationAndClearsRegeneration(t *testing.T) {
	a := StreamAccumulator{ThinkingEnabled: true}
	a.Apply(sse.LineResult{Parts: []sse.ContentPart{{Type: "thinking", Text: `plan <|EPSE|inv`}}})
	a.ResetContentPolicy()
	a.Apply(sse.LineResult{Parts: []sse.ContentPart{{Type: "thinking", Text: `oke name="run"><|EPSE|parameter name="cmd">secret</|EPSE|parameter></|EPSE|invoke> done`}}})
	a.Apply(sse.LineResult{Flush: true})
	if a.Thinking.String() != "plan  done" {
		t.Fatalf("continuation lost guard state: %q", a.Thinking.String())
	}
	a.Apply(sse.LineResult{Parts: []sse.ContentPart{{Type: "thinking", Text: `<|EPSE|tool_calls>`}}})
	a.RawThinking.Reset()
	a.ResetThinkingGuard()
	a.Apply(sse.LineResult{Parts: []sse.ContentPart{{Type: "thinking", Text: "new reasoning"}}})
	if a.Thinking.String() != "plan  donenew reasoning" {
		t.Fatalf("failed generation's capture contaminated replacement: %q", a.Thinking.String())
	}
}
