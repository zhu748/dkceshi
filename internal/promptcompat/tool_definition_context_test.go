package promptcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolDefinitionExtensionsSurviveEveryTextTransport(t *testing.T) {
	tool := map[string]any{"type": "function", "metadata": map[string]any{"note": "OUTER_CONTEXT", "encrypted_content": "BUSINESS_TEXT"}, "cache_control": "TRANSPORT_HINT", "description": "OUTER_DESCRIPTION", "function": map[string]any{
		"name": "write", "description": "  INNER_DESCRIPTION\r\n", "parameters": map[string]any{"type": "object"}, "usage_policy": "CONFIRM_BEFORE_OVERWRITE", "nullable": nil, "sequence": json.Number("9007199254740993"), "attachment": map[string]any{"type": "image", "data": "BINARY_SENTINEL"},
	}}
	tools := []any{tool}
	before, _ := json.Marshal(tools)
	req := map[string]any{"model": "deepseek-v4.1-flash", "tools": tools, "messages": []any{map[string]any{"role": "user", "content": "TASK"}}}
	std, err := NormalizeOpenAIChatRequest(nil, req, "", "CALLER_TAG")
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NormalizeOpenAIChatRequest(nil, std.OpenAIChatRequest(req), "", "CALLER_TAG")
	if err != nil {
		t.Fatal(err)
	}
	file, _ := BuildOpenAIToolsContextTranscript(tools, std.ToolChoice)
	for _, text := range []string{std.FinalPrompt, bridge.FinalPrompt, file} {
		for _, want := range []string{"OUTER_CONTEXT", "BUSINESS_TEXT", "OUTER_DESCRIPTION", `  INNER_DESCRIPTION\r\n`, "CONFIRM_BEFORE_OVERWRITE", `"nullable":null`, "9007199254740993"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("context lost or duplicated: %q in %s", want, text)
			}
		}
		if strings.Contains(text, "TRANSPORT_HINT") || strings.Contains(text, "BINARY_SENTINEL") {
			t.Fatal("binary or transport state entered catalog text")
		}
	}
	after, _ := json.Marshal(tools)
	if string(before) != string(after) {
		t.Fatal("catalog rendering mutated the caller's tools")
	}
	fields := std.ContextManifest().ToolFields
	found := map[string]bool{}
	for _, field := range fields {
		if field.Path == "/tools/0/function/usage_policy" {
			found["policy"] = field.Disposition == "tool_context" && reflect.DeepEqual(field.Destinations, []string{"live_prompt"})
		}
		if field.Path == "/tools/0/cache_control" {
			found["transport"] = field.OmittedReason == "transport_hint" && len(field.Destinations) == 0
		}
		for _, path := range field.OmittedPaths {
			found["binary"] = found["binary"] || path == "/tools/0/function/attachment/data"
		}
	}
	if !found["policy"] || !found["transport"] || !found["binary"] {
		t.Fatalf("missing field transport evidence: %+v", fields)
	}
}

func TestToolDefinitionPolicyAndDescriptionWhitespace(t *testing.T) {
	tools := []any{map[string]any{"name": "write", "description": "  EXACT\r\n", "usage_policy": "PRIVATE_TOOL_RULE"}}
	text, _ := BuildOpenAIToolsContextTranscript(tools, DefaultToolChoicePolicy())
	if !strings.Contains(text, "Description:   EXACT\r\n") {
		t.Fatal("description whitespace changed")
	}
	for _, policy := range []ToolChoicePolicy{{Mode: ToolChoiceNone}, {RestrictAllowed: true, Allowed: map[string]struct{}{"read": {}}}} {
		text, _ := BuildOpenAIToolsContextTranscript(tools, policy)
		if strings.Contains(text, "PRIVATE_TOOL_RULE") {
			t.Fatal("excluded catalog context entered text")
		}
		for _, field := range (StandardRequest{ToolsRaw: tools, ToolChoice: policy}).ContextManifest().ToolFields {
			if len(field.Destinations) != 0 || field.OmittedReason == "" {
				t.Fatal("excluded tool has an unexplained field destination")
			}
		}
	}
}

func TestToolFieldTraceIsBoundedWithoutTrimmingCatalogText(t *testing.T) {
	tool := map[string]any{"name": "write"}
	for i := 0; i <= maxInputSourceFields; i++ {
		tool["context_"+string(rune(0x4e00+i))] = "VALUE"
	}
	std := StandardRequest{ToolsRaw: []any{tool}}
	manifest := std.ContextManifest()
	if len(manifest.ToolFields) > maxInputSourceFields+1 || manifest.ToolFields[len(manifest.ToolFields)-1].Disposition != "adapter_source_trace_limit" {
		t.Fatal("tool field trace did not enforce its budget")
	}
	text, _ := BuildOpenAIToolsContextTranscript(std.ToolsRaw, DefaultToolChoicePolicy())
	if strings.Count(text, "VALUE") != maxInputSourceFields+1 {
		t.Fatal("trace budget truncated actual context")
	}
}
