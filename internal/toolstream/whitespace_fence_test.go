package toolstream

import (
	"strings"
	"testing"
)

func TestWhitespaceChunksPreserveFenceBoundariesBeforeToolCalls(t *testing.T) {
	const call = `<|EPSE|tool_calls><|EPSE|invoke name="edit"><|EPSE|parameter name="file_path"><![CDATA[a.py]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, fence := range []string{"```", "~~~"} {
		for _, whitespace := range []string{"\n", "\n\n", "\r\n", "\n  ", "\n\t"} {
			t.Run(fence+"/"+whitespace, func(t *testing.T) {
				state := State{DeferMalformed: true}
				chunks := []string{"Evidence:", whitespace, fence + "text\n", "sample\n", fence, "\n\n", "Next operation:\n", call}
				var visible strings.Builder
				calls := 0
				for _, chunk := range chunks {
					for _, event := range ProcessChunk(&state, chunk, []string{"edit"}) {
						if !event.Deferred {
							visible.WriteString(event.Content)
						}
						calls += len(event.ToolCalls)
					}
				}
				for _, event := range Flush(&state, []string{"edit"}) {
					if !event.Deferred {
						visible.WriteString(event.Content)
					}
					calls += len(event.ToolCalls)
				}
				want := strings.Join(chunks[:len(chunks)-1], "")
				if calls != 1 || visible.String() != want {
					t.Fatalf("call leaked after a closed fence: calls=%d visible=%q want=%q", calls, visible.String(), want)
				}
			})
		}
	}
}

func TestWhitespaceChunksKeepFencedToolExamplesLiteral(t *testing.T) {
	const example = `<|EPSE|tool_calls><|EPSE|invoke name="edit"></|EPSE|invoke></|EPSE|tool_calls>`
	chunks := []string{"Example:", "\n\n", "```xml\n", example, "\n", "```", "\n", "End."}
	state := State{DeferMalformed: true}
	var visible strings.Builder
	var events []Event
	for _, chunk := range chunks {
		events = append(events, ProcessChunk(&state, chunk, []string{"edit"})...)
	}
	for _, event := range append(events, Flush(&state, []string{"edit"})...) {
		if len(event.ToolCalls) != 0 {
			t.Fatal("a fenced example became an executable call")
		}
		visible.WriteString(event.Content)
	}
	if visible.String() != strings.Join(chunks, "") {
		t.Fatalf("fenced example was changed: %q", visible.String())
	}
}
