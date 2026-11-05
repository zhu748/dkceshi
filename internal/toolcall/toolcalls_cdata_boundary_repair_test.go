package toolcall

import (
	"strings"
	"testing"
)

// swallowedNextParameterBadCode is the shape reported from a live session: the
// `command` parameter's CDATA has no `]]>` of its own, so the unbounded
// findToolCDATAEnd search runs past the element and terminates on the `]]>` of
// the FOLLOWING `description` parameter. Before the element-boundary repair the
// parser returned a single-parameter call whose `command` value carried
// `</|EPSE|parameter><|EPSE|parameter name="description"><![CDATA[...` — the
// tool-call framing leaked into a parameter value and `description` vanished.
const swallowedNextParameterBadCode = `<|EPSE|tool_calls>
  <|EPSE|invoke name="pwsh">
    <|EPSE|parameter name="command"><![CDATA[$files = Get-ChildItem -Recurse -Filter *.go -File
$rx = [regex]'(?:errors\.New|fmt\.Errorf)\s*\(\s*"([^"]*)"'
Write-Host "TOTAL: $($all.Count)"</|EPSE|parameter>
    <|EPSE|parameter name="description"><![CDATA[Extract and count duplicate error message strings in Go files]]></|EPSE|parameter>
  </|EPSE|invoke>
</|EPSE|tool_calls>`

// The old recovery inferred extra parameters from a syntactically closed
// literal. The same bytes can be valid code/documentation; preserve them and
// let required-field validation trigger corrective generation when necessary.
func TestAmbiguousClosedCDATAPreservesOriginalValue(t *testing.T) {
	calls := ParseToolCalls(swallowedNextParameterBadCode, nil)
	start := strings.Index(swallowedNextParameterBadCode, "<![CDATA[") + len("<![CDATA[")
	end := strings.LastIndex(swallowedNextParameterBadCode, "]]>")
	if len(calls) != 1 || len(calls[0].Input) != 1 || calls[0].Input["command"] != swallowedNextParameterBadCode[start:end] {
		t.Fatalf("closed literal was reinterpreted: %#v", calls)
	}
}

func TestConsecutiveCDATAExamplesDoNotCreateParameters(t *testing.T) {
	value := `echo one</|EPSE|parameter><|EPSE|parameter name="description"><![CDATA[two</|EPSE|parameter><|EPSE|parameter name="workdir"><![CDATA[/tmp`
	text := `<|EPSE|tool_calls><|EPSE|invoke name="Bash"><|EPSE|parameter name="command"><![CDATA[` + value + `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	calls := ParseToolCalls(text, nil)
	if len(calls) != 1 || len(calls[0].Input) != 1 || calls[0].Input["command"] != value {
		t.Fatalf("literal changed: %#v", calls)
	}
}

// TestQuotedToolCallExampleKeepsGreedyCDATAInterpretation is the negative that
// bounds the repair. A Write payload whose value quotes a complete tool-call
// example also contains `<|EPSE|parameter ...><![CDATA[` inside its CDATA body,
// but it never escaped its own element, so the value must come back byte-exact.
func TestQuotedToolCallExampleKeepsGreedyCDATAInterpretation(t *testing.T) {
	content := strings.Join([]string{
		"# How to call a tool",
		"",
		"```xml",
		`<|EPSE|tool_calls>`,
		`  <|EPSE|invoke name="Bash">`,
		`    <|EPSE|parameter name="command"><![CDATA[echo hi]]></|EPSE|parameter>`,
		`  </|EPSE|invoke>`,
		`</|EPSE|tool_calls>`,
		"```",
	}, "\n")
	text := `<|EPSE|tool_calls><|EPSE|invoke name="Write">` +
		`<|EPSE|parameter name="content"><![CDATA[` + content + `]]></|EPSE|parameter>` +
		`<|EPSE|parameter name="file_path"><![CDATA[docs/tools.md]]></|EPSE|parameter>` +
		`</|EPSE|invoke></|EPSE|tool_calls>`

	calls := ParseToolCalls(text, nil)
	if len(calls) != 1 || len(calls[0].Input) != 2 {
		t.Fatalf("expected one call with two parameters, got %#v", calls)
	}
	if got, _ := calls[0].Input["content"].(string); got != content {
		t.Fatalf("quoted tool-call example was mutated:\n got: %q\nwant: %q", got, content)
	}
	if got, _ := calls[0].Input["file_path"].(string); got != "docs/tools.md" {
		t.Fatalf("file_path = %q, want docs/tools.md", got)
	}
}

// TestProseCDATAMentionKeepsGreedyCDATAInterpretation is the second negative: a
// value that mentions a bare unclosed `<![CDATA[` in prose has an unbalanced
// opener but no preceding close tag, so the repair must not fire.
func TestProseCDATAMentionKeepsGreedyCDATAInterpretation(t *testing.T) {
	content := "the <![CDATA[ opener is what protects a long value"
	text := `<|EPSE|tool_calls><|EPSE|invoke name="Write">` +
		`<|EPSE|parameter name="content"><![CDATA[` + content + `]]></|EPSE|parameter>` +
		`<|EPSE|parameter name="file_path"><![CDATA[notes.md]]></|EPSE|parameter>` +
		`</|EPSE|invoke></|EPSE|tool_calls>`

	calls := ParseToolCalls(text, nil)
	if len(calls) != 1 || len(calls[0].Input) != 2 {
		t.Fatalf("expected one call with two parameters, got %#v", calls)
	}
	if got, _ := calls[0].Input["content"].(string); got != content {
		t.Fatalf("prose CDATA mention was mutated:\n got: %q\nwant: %q", got, content)
	}
}

// TestSwallowedCDATARepairLeavesWellFormedMarkupUntouched pins the no-op path:
// the repair must be byte-identical on input it has no business changing.
func TestSwallowedCDATARepairLeavesWellFormedMarkupUntouched(t *testing.T) {
	text := `<|EPSE|tool_calls><|EPSE|invoke name="Bash">` +
		`<|EPSE|parameter name="command"><![CDATA[echo hi]]></|EPSE|parameter>` +
		`<|EPSE|parameter name="description"><![CDATA[greet]]></|EPSE|parameter>` +
		`</|EPSE|invoke></|EPSE|tool_calls>`
	calls := ParseToolCalls(text, nil)
	if len(calls) != 1 || calls[0].Input["command"] != "echo hi" || calls[0].Input["description"] != "greet" {
		t.Fatalf("well-formed values changed: %#v", calls)
	}
}

// TestSwallowedCDATARepairKeepsIntentFallbackForUnrepairableCode pins that the
// repair does not shadow the LLM fallback: input it cannot improve still reports
// zero calls plus a tool-call intent, which is what routes to repair.
func TestSwallowedCDATARepairKeepsIntentFallbackForUnrepairableCode(t *testing.T) {
	parsed := parseToolCallsDetailedXMLOnly(realWorldSwallowedStructureBadCode)
	if len(parsed.Calls) != 0 {
		t.Fatalf("call count = %d, want 0 (input is not deterministically repairable)", len(parsed.Calls))
	}
	if !parsed.SawToolCallIntent {
		t.Fatal("SawToolCallIntent = false, want true so the LLM repair path still runs")
	}
}
