package toolcall

import (
	"context"
	"testing"
)

func TestRepairRejectsUnresolvedSiblingWithoutClosingTag(t *testing.T) {
	valid := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	for _, tail := range []string{`<|EPSE|tool_calls>`, `<tool_calls>`, `<|OTHER|broken>`, `<|EPSE|invoke name="write"`} {
		t.Run(tail, func(t *testing.T) {
			calls, ok := RepairToolCallsWithLLM(context.Background(), valid, func(context.Context, string) (string, error) { return valid + tail, nil })
			if ok || len(calls) != 0 {
				t.Fatalf("partial repair accepted: %#v", calls)
			}
		})
	}
}
