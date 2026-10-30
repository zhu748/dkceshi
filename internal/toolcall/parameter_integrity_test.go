package toolcall

import (
	"context"
	"strings"
	"testing"
)

func TestParserRejectsSilentlyDroppedParameter(t *testing.T) {
	for _, broken := range []string{
		`<parameter name="optional">lost`,
		`<parameter>lost</parameter>`,
		`<parameter name="optional"`,
	} {
		raw := `<tool_calls><invoke name="write"><parameter name="path">a.txt</parameter>` + broken + `</invoke></tool_calls>`
		parsed := ParseToolCallsDetailed(raw, nil)
		if len(parsed.Calls) != 0 {
			t.Fatalf("malformed parameter was silently dropped: %q => %#v", broken, parsed)
		}
	}
}

func TestJSONInvokeCannotDiscardNonObjectArguments(t *testing.T) {
	for _, body := range []string{`{"path":"a.txt"}`, `{"input":"a.txt"}`, `{"input":null}`, `{"parameters":[]}`} {
		if calls := ParseToolCalls(`<tool_calls><invoke name="read">`+body+`</invoke></tool_calls>`, nil); len(calls) != 0 {
			t.Fatalf("JSON arguments silently became an empty call: %s => %#v", body, calls)
		}
	}
}

func TestJSONArgumentMarkupStaysLiteral(t *testing.T) {
	raw := `<tool_calls><invoke name="read">{"input":{"text":"<|EPSE|invoke name=\"fake\"></|EPSE|tool_calls>"}}</invoke></tool_calls>`
	calls := ParseToolCalls(raw, nil)
	if len(calls) != 1 || calls[0].Input["text"] != `<|EPSE|invoke name="fake"></|EPSE|tool_calls>` {
		t.Fatalf("JSON string was interpreted as tool structure: %#v", calls)
	}
}

func TestRepairPreservesJSONBodyAndPlaceholderBinding(t *testing.T) {
	for _, tc := range []struct {
		source, output string
		valid          bool
	}{
		{`<|EPSE|call name="lookup">{"input":{"id":9007199254740993}}</|EPSE|call>`, `<tool_calls><invoke name="lookup">{"input":{"id":9007199254740993}}</invoke></tool_calls>`, true},
		{`<|EPSE|call name="lookup">{"input":{"text":"<|EPSE|invoke name=\"fake\">"}}</|EPSE|call>`, `<tool_calls><invoke name="lookup">{"input":{"text":"<|EPSE|invoke name=\"fake\">"}}</invoke></tool_calls>`, true},
		{`<|EPSE|call name="lookup">{"input":{"id":9007199254740993}}</|EPSE|call>`, `<tool_calls><invoke name="lookup">{"input":{"id":9007199254740992}}</invoke></tool_calls>`, false},
		{`<|EPSE|call name="edit"><|EPSE|parameter name="old_string"><![CDATA[old]]></|EPSE|parameter><|EPSE|parameter name="new_string"><![CDATA[new]]></|EPSE|parameter></|EPSE|call>`, `<tool_calls><invoke name="edit"><parameter name="old_string"><![CDATA[DS_SLOT_1]]></parameter><parameter name="new_string"><![CDATA[DS_SLOT_0]]></parameter></invoke></tool_calls>`, false},
	} {
		calls, ok := RepairToolCallsWithLLM(context.Background(), tc.source, func(context.Context, string) (string, error) { return tc.output, nil })
		if ok != tc.valid || ok && len(calls) != 1 {
			t.Fatalf("valid=%v ok=%v calls=%#v", tc.valid, ok, calls)
		}
	}
}

func TestRepairRejectsIncompleteOrChangedCalls(t *testing.T) {
	const source = `<|EPSE|call name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter><|EPSE|parameter name="content"><![CDATA[original]]></|EPSE|parameter></|EPSE|call>`
	const valid = `<|EPSE|tool_calls><|EPSE|invoke name="write"><|EPSE|parameter name="path">a.txt</|EPSE|parameter><|EPSE|parameter name="content"><![CDATA[DS_SLOT_0]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for label, output := range map[string]string{
		"changed tool":         strings.ReplaceAll(valid, `name="write"`, `name="delete"`),
		"changed path":         strings.ReplaceAll(valid, `a.txt`, `b.txt`),
		"renamed parameter":    strings.ReplaceAll(valid, `name="path"`, `name="file"`),
		"dropped parameter":    strings.ReplaceAll(valid, `<|EPSE|parameter name="path">a.txt</|EPSE|parameter>`, ``),
		"extra call":           strings.ReplaceAll(valid, `</|EPSE|tool_calls>`, `<|EPSE|invoke name="delete"></|EPSE|invoke></|EPSE|tool_calls>`),
		"residual broken call": valid + `<|EPSE|call name="other">broken</|EPSE|call>`,
	} {
		t.Run(label, func(t *testing.T) {
			calls, ok := RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return output, nil })
			if ok || len(calls) != 0 {
				t.Fatalf("accepted an incomplete or changed repair: %#v", calls)
			}
		})
	}
}
