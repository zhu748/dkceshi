package toolcall

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestFieldNamedParameterShorthand(t *testing.T) {
	const value = "\n  原文 <|EPSE|description\"> & <tag>\r\n  "
	for _, quote := range []string{`"`, `'`} {
		raw := `<|EPSE|tool_calls><|EPSE|invoke name="edit"><|EPSE|parameter name="file_path"><![CDATA[a.txt]]></|EPSE|parameter>` +
			`<|EPSE|old_string` + quote + `><![CDATA[` + value + `]]></|EPSE|parameter>` +
			`<|EPSE|new_string` + quote + `><![CDATA[]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
		parsed := ParseToolCallsDetailed(raw, []string{"edit"})
		want := []ParsedToolCall{{Name: "edit", Input: map[string]any{"file_path": "a.txt", "old_string": value, "new_string": ""}}}
		if !reflect.DeepEqual(parsed.Calls, want) || parsed.SawToolCallIntent {
			t.Fatalf("shorthand lost source parameters: %#v", parsed)
		}
	}
}

func TestFieldNamedParameterShorthandMustClose(t *testing.T) {
	raw := `<|EPSE|tool_calls><|EPSE|invoke name="pwsh"><|EPSE|parameter name="command">Get-Date</|EPSE|parameter><|EPSE|description"><![CDATA[lost]]></|EPSE|invoke></|EPSE|tool_calls>`
	if calls := ParseToolCalls(raw, nil); len(calls) != 0 {
		t.Fatalf("unclosed optional shorthand was silently discarded: %#v", calls)
	}
}

func TestFieldNamedParameterShorthandLiteralStaysLiteral(t *testing.T) {
	const literal = `<|EPSE|description"><![CDATA[example]]></|EPSE|parameter>`
	for _, text := range []string{"`" + literal + "`", "```xml\n" + literal + "\n```", `{"example":"<|EPSE|description\">"}`} {
		if got := canonicalizeToolCallCandidateSpans(text); got != text {
			t.Fatalf("literal markup changed: %q", got)
		}
	}
	// A plain XML tag is not evidence of a tool parameter shorthand.
	if _, ok := scanToolMarkupTagAt(`<description">`, 0); ok {
		t.Fatal("recognized non-tool markup as a shorthand parameter")
	}
}

func TestRepairBindsFieldNamedParameterShorthand(t *testing.T) {
	const source = `<|EPSE|call name="pwsh"><|EPSE|parameter name="command"><![CDATA[Get-Date]]></|EPSE|parameter><|EPSE|description"><![CDATA[Show date]]></|EPSE|parameter></|EPSE|call>`
	const output = `<|EPSE|tool_calls><|EPSE|invoke name="pwsh"><|EPSE|parameter name="command"><![CDATA[DS_SLOT_0]]></|EPSE|parameter><|EPSE|parameter name="description"><![CDATA[DS_SLOT_1]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, rename := range []bool{false, true} {
		repaired := output
		if rename {
			repaired = strings.Replace(repaired, `name="description"`, `name="other"`, 1)
		}
		calls, ok := RepairToolCallsWithLLM(t.Context(), source, func(context.Context, string) (string, error) { return repaired, nil })
		if ok == rename || ok && (len(calls) != 1 || calls[0].Input["description"] != "Show date") {
			t.Fatalf("rename=%v: repair identity check failed: ok=%v calls=%#v", rename, ok, calls)
		}
	}
}
