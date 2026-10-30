package toolstream

import (
	"testing"

	"ds2api/internal/toolcall"
)

func TestFieldNamedParameterShorthandAcrossStreamSplits(t *testing.T) {
	const raw = `<|EPSE|tool_calls><|EPSE|invoke name="pwsh"><|EPSE|parameter name="command"><![CDATA[Get-Date]]></|EPSE|parameter><|EPSE|description"><![CDATA[Show date]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for split := 0; split <= len(raw); split++ {
		var state State
		events := ProcessChunk(&state, raw[:split], []string{"pwsh"})
		events = append(events, ProcessChunk(&state, raw[split:], []string{"pwsh"})...)
		events = append(events, Flush(&state, []string{"pwsh"})...)
		var calls []toolcall.ParsedToolCall
		for _, event := range events {
			if event.Content != "" {
				t.Fatalf("split %d leaked tool markup: %q", split, event.Content)
			}
			calls = append(calls, event.ToolCalls...)
		}
		if len(calls) != 1 || calls[0].Input["description"] != "Show date" {
			t.Fatalf("split %d lost a parameter: %#v", split, calls)
		}
	}
}
