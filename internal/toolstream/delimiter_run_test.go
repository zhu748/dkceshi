package toolstream

import (
	"strings"
	"testing"
)

func TestLargeDelimiterRunRetainsTextAcrossStreamPartitions(t *testing.T) {
	raw := strings.Repeat("<", 131072) + ">"
	for _, size := range []int{len(raw), 257} {
		state := &State{DeferMalformed: true, CleanVisible: true}
		var events []Event
		for start := 0; start < len(raw); start += size {
			events = append(events, ProcessChunk(state, raw[start:min(len(raw), start+size)], nil)...)
		}
		events = append(events, Flush(state, nil)...)
		var text strings.Builder
		for _, event := range events {
			if len(event.ToolCalls) > 0 {
				t.Fatal("ordinary delimiters became executable calls")
			}
			text.WriteString(event.Content)
		}
		if text.String() != raw {
			t.Fatalf("delimiter text changed at chunk size %d", size)
		}
	}
}
