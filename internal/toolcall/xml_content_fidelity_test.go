package toolcall

import (
	"context"
	"testing"
)

func TestXMLContextWithNoLosslessObjectMappingStaysLiteral(t *testing.T) {
	for _, body := range []string{
		`<value>HEAD<part>FIRST</part>MIDDLE<part>SECOND</part>TAIL</value>`,
		`HEAD<part>FIRST</part>TAIL`,
		`<value><!-- DO NOT LOSE --><part>FIRST</part></value>`,
		`<value><?instruction KEEP?><part>FIRST</part></value>`,
		`<value><![CDATA[HEAD]]><part>FIRST</part>TAIL</value>`,
	} {
		raw := `<tool_calls><invoke name="write"><parameter name="payload">` + body + `</parameter></invoke></tool_calls>`
		calls := ParseToolCalls(raw, nil)
		if len(calls) != 1 || calls[0].Input["payload"] != body {
			t.Fatalf("XML context reduced to an object: body=%s calls=%+v", body, calls)
		}
		tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"properties": map[string]any{"payload": map[string]any{"type": "object"}}}}}
		if err := ValidateToolArguments(NormalizeParsedToolCallsForSchemas(calls, tools), tools); err == nil {
			t.Fatal("lossy XML object became executable")
		}
	}
}

func TestFormatRepairCannotRemoveXMLContext(t *testing.T) {
	for _, body := range []string{`<value>HEAD<part>FIRST</part>TAIL</value>`, `<value><!-- SOURCE --><part>FIRST</part></value>`} {
		source := `<|EPSE|call name="write"><parameter name="payload">` + body + `</parameter></|EPSE|call>`
		for _, changed := range []bool{false, true} {
			fixedBody := body
			if changed {
				fixedBody = `<value><part>FIRST</part></value>`
			}
			fixed := `<tool_calls><invoke name="write"><parameter name="payload">` + fixedBody + `</parameter></invoke></tool_calls>`
			calls, ok := RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return fixed, nil })
			if changed {
				if ok || len(calls) != 0 {
					t.Fatal("format repair removed source XML context")
				}
			} else if !ok || len(calls) != 1 || calls[0].Input["payload"] != body {
				t.Fatal("unchanged XML context could not be repaired")
			}
		}
	}
}
