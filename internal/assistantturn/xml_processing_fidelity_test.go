package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestXMLInstructionsRemainLiteralThroughValidationAndRepair(t *testing.T) {
	for _, value := range []string{
		"  <?keep ORIGINAL?>payload\r\n ",
		`<value><?keep ORIGINAL?>payload</value>`,
		`<!DOCTYPE keep>payload`,
		`<value><!DOCTYPE keep>payload</value>`,
	} {
		for _, typ := range []string{"string", "object"} {
			tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"data": map[string]any{"type": typ}}, "required": []any{"data"}}}}
			raw := `<tool_calls><invoke name="write"><parameter name="data">` + value + `</parameter></invoke></tool_calls>`
			opts := BuildOptions{ToolsRaw: tools}
			turns := []Turn{BuildTurnFromCollected(sse.CollectResult{Text: raw}, opts), BuildTurnFromStreamSnapshot(StreamSnapshot{RawText: raw}, opts)}
			for _, turn := range turns {
				if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["data"] != value {
					t.Fatalf("source changed: %q -> %+v", value, turn.ToolCalls)
				}
				if (turn.Error != nil) != (typ == "object") {
					t.Fatalf("literal XML must satisfy the declared type: %s %+v", typ, turn.Error)
				}
			}
		}
		source := `<|EPSE|broken><|EPSE|invoke name="write"><|EPSE|parameter name="data">` + value + `</|EPSE|parameter></|EPSE|invoke></|EPSE|broken>`
		for _, changed := range []bool{false, true} {
			replyValue := value
			if changed {
				replyValue = "payload"
			}
			reply := `<tool_calls><invoke name="write"><parameter name="data">` + replyValue + `</parameter></invoke></tool_calls>`
			calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return reply, nil })
			if ok == changed || ok && (len(calls) != 1 || calls[0].Input["data"] != value) {
				t.Fatalf("repair changed=%v accepted=%v calls=%+v", changed, ok, calls)
			}
		}
	}
}

func TestXMLInstructionStringsInsideJSONAndCDATAStayData(t *testing.T) {
	for _, value := range []string{`"<?keep ORIGINAL?>payload"`, `<![CDATA[<?keep ORIGINAL?>payload]]>`} {
		raw := `<tool_calls><invoke name="write"><parameter name="data">` + value + `</parameter></invoke></tool_calls>`
		calls := toolcall.NormalizeParsedToolCallsForSchemas(toolcall.ParseToolCalls(raw, nil), nil)
		if len(calls) != 1 || !strings.Contains(calls[0].Input["data"].(string), "<?keep ORIGINAL?>") {
			t.Fatalf("literal changed: %+v", calls)
		}
	}
}
