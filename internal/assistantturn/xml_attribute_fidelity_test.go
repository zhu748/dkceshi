package assistantturn

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestXMLAttributesRemainLiteralOrFailSchemaBeforeDelivery(t *testing.T) {
	for _, value := range []string{
		`<record id="SOURCE_ID">body &amp; text</record>`,
		`<outer><record id='SOURCE_ID'>body</record></outer>`,
		`<ns:record>body</ns:record>`,
	} {
		for _, cdata := range []bool{false, true} {
			body := value
			if cdata {
				body = "<![CDATA[" + body + "]]>"
			}
			raw := `<tool_calls><invoke name="save"><parameter name="payload">` + body + `</parameter></invoke></tool_calls>`
			for _, typ := range []string{"string", "object"} {
				tools := []any{map[string]any{"name": "save", "parameters": map[string]any{"type": "object", "properties": map[string]any{"payload": map[string]any{"type": typ}}}}}
				turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{ToolsRaw: tools})
				if typ == "string" {
					if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input["payload"] != value {
						t.Fatalf("literal XML changed: body=%s turn=%+v", body, turn)
					}
				} else if turn.Error == nil || turn.Error.Code != "invalid_tool_arguments" || !FinalizeTurn(turn, FinalizeOptions{}).ShouldFail {
					t.Fatalf("lossy XML object became deliverable: body=%s turn=%+v", body, turn)
				}
			}
		}
	}
}

func TestFormatRepairCannotDiscardNestedXMLAttributes(t *testing.T) {
	source := `<|EPSE|call name="save"><|EPSE|parameter name="payload"><record id="SOURCE_ID">body</record></|EPSE|parameter></|EPSE|call>`
	_, ok := toolcall.RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) {
		return `<tool_calls><invoke name="save"><parameter name="payload"><record>body</record></parameter></invoke></tool_calls>`, nil
	})
	if ok {
		t.Fatal("format repair erased a source attribute")
	}
}

func TestXMLAttributeExamplesInsideJSONStayStructured(t *testing.T) {
	raw := `<tool_calls><invoke name="save"><parameter name="payload">{"example":"<record id=\"SOURCE_ID\">body</record>"}</parameter></invoke></tool_calls>`
	calls := toolcall.ParseToolCalls(raw, nil)
	if len(calls) != 1 || !strings.Contains(calls[0].Input["payload"].(map[string]any)["example"].(string), "SOURCE_ID") {
		t.Fatal("JSON string example was interpreted as XML structure")
	}
}
