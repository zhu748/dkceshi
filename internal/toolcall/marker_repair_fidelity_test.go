package toolcall

import (
	"context"
	"strings"
	"testing"
)

func TestFormatRepairPreservesJSONMarkerLiteralsAndRejectsTheirMutation(t *testing.T) {
	const literal = "<|Q7ZK3M|invoke>"
	const payload = `{"snippet":"` + literal + `"}`
	const good = `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="payload">` + payload + `</|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	bad := strings.ReplaceAll(strings.ReplaceAll(good, `|invoke name=`, `|call name=`), `</|Q7ZK3M|invoke>`, `</|Q7ZK3M|call>`)
	for _, changed := range []bool{false, true} {
		called := false
		calls, ok := RepairToolCallsWithLLM(t.Context(), bad, func(_ context.Context, prompt string) (string, error) {
			called = true
			if !strings.Contains(prompt, payload) {
				t.Fatal("repair prompt modified the original JSON string")
			}
			if changed {
				return strings.Replace(good, payload, `{"snippet":"<|EPSE|invoke>"}`, 1), nil
			}
			return good, nil
		}, "Q7ZK3M")
		if !called || ok == changed || changed && len(calls) != 0 {
			t.Fatalf("repair accepted mutation or rejected faithful output: changed=%v ok=%v calls=%+v", changed, ok, calls)
		}
		if !changed {
			if len(calls) != 1 || calls[0].Input["payload"].(map[string]any)["snippet"] != literal {
				t.Fatalf("repair changed literal parameters: %+v", calls)
			}
		}
	}
}
