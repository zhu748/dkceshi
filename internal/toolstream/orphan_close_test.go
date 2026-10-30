package toolstream

import (
	"strings"
	"testing"

	"ds2api/internal/toolcall"
)

func TestOrphanToolClosesAcrossEveryChunkSize(t *testing.T) {
	call := `<|EPSE|tool_calls><|EPSE|invoke name="read"><|EPSE|parameter name="file_path"><![CDATA[a.txt]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, tc := range []struct {
		input string
		text  string
		calls int
		value string
	}{
		{"before\n</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>\nafter", "before\n\nafter", 0, ""},
		{call + "</|EPSE|tool_calls>\ndone", "\ndone", 1, "a.txt"},
		{"</|EPSE|tool_calls>" + call, "", 1, "a.txt"},
		{"before\n</｜DSML｜calls>\nafter", "before\n\nafter", 0, ""},
		{"before\n</｜｜DSML｜｜invoke>\nafter", "before\n\nafter", 0, ""},
		{"</｜DSML｜calls>" + call, "", 1, "a.txt"},
		{"`</|EPSE|tool_calls>`", "`</|EPSE|tool_calls>`", 0, ""},
		{"```xml\n</|EPSE|tool_calls>\n```\n", "```xml\n</|EPSE|tool_calls>\n```\n", 0, ""},
		{strings.ReplaceAll(call, "a.txt", "</|EPSE|tool_calls>"), "", 1, "</|EPSE|tool_calls>"},
	} {
		for size := 1; size <= len(tc.input); size++ {
			state := State{DeferMalformed: true}
			var events []Event
			for i := 0; i < len(tc.input); i += size {
				events = append(events, ProcessChunk(&state, tc.input[i:min(i+size, len(tc.input))], []string{"read"})...)
			}
			events = append(events, Flush(&state, []string{"read"})...)
			var text strings.Builder
			var calls []toolcall.ParsedToolCall
			for _, event := range events {
				text.WriteString(event.Content)
				calls = append(calls, event.ToolCalls...)
			}
			if text.String() != tc.text || len(calls) != tc.calls || (tc.calls > 0 && calls[0].Input["file_path"] != tc.value) {
				t.Fatalf("size=%d input=%q text=%q calls=%#v", size, tc.input, text.String(), calls)
			}
		}
	}
}
