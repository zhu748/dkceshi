package toolcall_test

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/prompt"
	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

func TestToolParameterNamesRoundTripThroughHistoryAndValidation(t *testing.T) {
	for _, key := range []string{" key ", "", " \t ", "中文字段"} {
		history := []any{map[string]any{"name": "check", "input": map[string]any{key: "  exact\r\n"}}}
		raw := prompt.FormatToolCallsForPrompt(history)
		tools := []any{map[string]any{"name": "check", "parameters": map[string]any{"type": "object", "properties": map[string]any{key: map[string]any{"type": "string"}}, "required": []any{key}, "additionalProperties": false}}}
		turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolsRaw: tools})
		if turn.Error != nil || len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Input[key] != "  exact\r\n" {
			t.Fatalf("parameter key %q changed: %+v, %#v", key, turn.Error, turn.ToolCalls)
		}
		source := `<|EPSE|call name="check"><parameter name="` + key + `">"v"</parameter></|EPSE|call>`
		fixed := `<tool_calls><invoke name="check"><parameter name="` + key + `">"v"</parameter></invoke></tool_calls>`
		calls, ok := toolcall.RepairToolCallsWithLLM(context.Background(), source, func(context.Context, string) (string, error) { return fixed, nil })
		if !ok || len(calls) != 1 || calls[0].Input[key] != "v" {
			t.Fatalf("repair changed key %q: %#v", key, calls)
		}
	}
}

func TestToolUnicodeRejectsJSONButKeepsCDATAExamplesLiteral(t *testing.T) {
	for _, body := range []string{`"\ud800"`, `{"nested":"\udc00"}`} {
		raw := `<tool_calls><invoke name="check"><parameter name="value">` + body + `</parameter></invoke></tool_calls>`
		turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{})
		if turn.Error == nil || len(turn.ToolCalls) != 0 {
			t.Fatalf("invalid JSON escaped Unicode delivered: %#v", turn)
		}
		raw = `<tool_calls><invoke name="check"><parameter name="value"><![CDATA[` + body + `]]></parameter></invoke></tool_calls>`
		calls := toolcall.NormalizeParsedToolCallsForSchemas(toolcall.ParseToolCalls(raw, nil), nil)
		if len(calls) != 1 || calls[0].Input["value"] != body {
			t.Fatal("literal JSON example changed")
		}
	}
}

func TestToolLimitsRejectWholeBatchAndIgnoreLiteralTags(t *testing.T) {
	t.Setenv("DS2API_STREAM_MAX_TOOL_CALLS", "2")
	for _, raw := range []string{
		"<tool_calls>" + strings.Repeat(`<invoke name="check">`, 6400) + "</tool_calls>",
		"<tool_calls>" + strings.Repeat(`<invoke name="check"></invoke>`, 3) + "</tool_calls>",
	} {
		parsed := toolcall.ParseToolCallsDetailed(raw, nil)
		if !parsed.ResourceLimitExceeded || len(parsed.Calls) != 0 {
			t.Fatalf("oversized structure became partial calls: %#v", parsed)
		}
		called := false
		turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolCallRepair: func(context.Context, string) (string, error) { called = true; return "", nil }})
		if called || turn.Error == nil {
			t.Fatal("over-limit batch reached repair or delivery")
		}
	}
	body := strings.Repeat(`<invoke name="literal">`, 6400)
	raw := `<tool_calls><invoke name="check"><parameter name="content"><![CDATA[` + body + `]]></parameter></invoke></tool_calls>`
	parsed := toolcall.ParseToolCallsDetailed(raw, nil)
	if parsed.ResourceLimitExceeded || len(parsed.Calls) != 1 || parsed.Calls[0].Input["content"] != body {
		t.Fatal("literal tool syntax consumed the structural budget")
	}
}

func BenchmarkMalformedToolBatch(b *testing.B) {
	raw := "<tool_calls>" + strings.Repeat(`<invoke name="check">`, 6400) + "</tool_calls>"
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		toolcall.ParseToolCallsDetailed(raw, nil)
	}
}

func TestNestedXMLBudgetDoesNotBecomeAnExecutablePartialValue(t *testing.T) {
	for _, name := range []string{"node", "中文", "ns:node"} {
		checkNestedXMLBudget(t, name)
	}
}

func checkNestedXMLBudget(t *testing.T, name string) {
	t.Helper()
	body := strings.Repeat("<"+name+">", 150) + "value" + strings.Repeat("</"+name+">", 150)
	raw := `<tool_calls><invoke name="check"><parameter name="payload">` + body + `</parameter></invoke></tool_calls>`
	called := false
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: raw}, assistantturn.BuildOptions{ToolCallRepair: func(context.Context, string) (string, error) { called = true; return raw, nil }})
	if turn.Error == nil || len(turn.ToolCalls) != 0 || called {
		t.Fatal("deep nested XML was parsed or sent to model repair")
	}
	raw = `<tool_calls><invoke name="check"><parameter name="payload"><![CDATA[` + body + `]]></parameter></invoke></tool_calls>`
	calls := toolcall.NormalizeParsedToolCallsForSchemas(toolcall.ParseToolCalls(raw, nil), nil)
	if len(calls) != 1 || calls[0].Input["payload"] != body {
		t.Fatal("deep literal XML text changed")
	}
}
