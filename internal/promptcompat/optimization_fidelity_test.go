package promptcompat

import (
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/prompt"
	"ds2api/internal/toolcall"
)

func TestAllRolesPreserveOrderedReasoningAndBusinessFieldText(t *testing.T) {
	for _, role := range []string{"user", "tool", "assistant", "system", "developer"} {
		content := []any{map[string]any{"type": "text", "text": "BEFORE"}, map[string]any{"type": "thinking", "thinking": "  THOUGHT\r\n", "annotations": []any{"SOURCE"}}, map[string]any{"type": "mcp_result", "payload": map[string]any{"file_data": "FILE_TEXT", "inline_data": map[string]any{"text": "INLINE_TEXT"}, "image_data": "IMAGE_TEXT", "audio_data": "AUDIO_TEXT"}}, map[string]any{"type": "text", "text": "AFTER"}}
		messages := []any{map[string]any{"role": role, "content": content}}
		std, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{std.FinalPrompt, BuildOpenAIHistoryTranscript(std.Messages)} {
			for _, want := range []string{"  THOUGHT\r\n", "SOURCE", "FILE_TEXT", "INLINE_TEXT", "IMAGE_TEXT", "AUDIO_TEXT"} {
				if strings.Count(text, want) != 1 {
					t.Fatalf("role=%s lost/duplicated %s: %s", role, want, text)
				}
			}
			if strings.Index(text, "BEFORE") > strings.Index(text, "THOUGHT") || strings.Index(text, "THOUGHT") > strings.Index(text, "AFTER") {
				t.Fatalf("role=%s moved content", role)
			}
		}
	}
}

func TestToolSchemaAliasesCannotWeakenValidation(t *testing.T) {
	strict := map[string]any{"type": "object", "required": []any{"path"}}
	for _, alias := range []string{"parameters", "input_schema", "inputSchema", "schema"} {
		for _, other := range []any{map[string]any{"type": "object"}, nil, false} {
			tools := []any{map[string]any{alias: other, "type": "function", "function": map[string]any{"name": "read", "parameters": strict}}}
			if toolcall.ValidateToolSchemas(tools) == nil || toolcall.ValidateToolArguments([]toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{}}}, tools) == nil {
				t.Fatalf("conflicting %s schema accepted", alias)
			}
		}
		tools := []any{map[string]any{alias: strict, "function": map[string]any{"name": "read", "parameters": strict}}}
		if err := toolcall.ValidateToolSchemas(tools); err != nil {
			t.Fatal(err)
		}
		if toolcall.ValidateToolArguments([]toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{}}}, tools) == nil {
			t.Fatal("matching schemas lost required properties")
		}
	}
}

func TestToolSelectorAliasesRejectConflictInForcedAndAllowedPolicies(t *testing.T) {
	tools := []any{map[string]any{"name": "read"}, map[string]any{"name": "delete"}}
	for _, selector := range []map[string]any{
		{"type": "function", "name": "delete", "function": map[string]any{"name": "read"}},
		{"type": "function", "name": 1, "function": map[string]any{"name": "read"}},
		{"type": "function", "name": "read", "function": "invalid"},
	} {
		for _, choice := range []any{selector, map[string]any{"type": "allowed_tools", "mode": "auto", "tools": []any{selector}}} {
			if _, err := ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": choice}); err == nil {
				t.Fatalf("ambiguous selector accepted: %+v", choice)
			}
		}
	}
	policy, err := ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": map[string]any{"type": "function", "name": "read", "function": map[string]any{"name": "read"}}})
	if err != nil || policy.ForcedName != "read" {
		t.Fatalf("matching aliases rejected: %+v %v", policy, err)
	}
}

func TestManifestTracksNestedRedactionsAndPreservesSourceSnapshot(t *testing.T) {
	r := StandardRequest{Messages: []any{map[string]any{"role": "user", "content": []any{
		map[string]any{"type": "mcp_result", "payload": map[string]any{"file_data": "BUSINESS", "a/b~c": map[string]any{"type": "image", "data": "PRIVATE_BYTES"}}},
		map[string]any{"type": "thinking", "thinking": "THOUGHT", "signature": "PRIVATE_SIGNATURE"},
	}}}}
	manifest := r.ContextManifest()
	want := []string{"/content/0/payload/a~1b~0c/data", "/content/1/signature"}
	if !reflect.DeepEqual(manifest.SourceMessages[0].OmittedPaths, want) || len(manifest.Omissions) != 2 {
		t.Fatalf("incorrect redaction evidence: %+v", manifest)
	}
	r.SourceContext = r.ContextMessageDigests()
	r.Messages = nil
	after := r.ContextManifest()
	if !reflect.DeepEqual(after.SourceMessages[0].OmittedPaths, want) {
		t.Fatal("externalization lost source evidence")
	}
	after.SourceMessages[0].OmittedPaths[0] = "mutated"
	if r.SourceContext[0].OmittedPaths[0] == "mutated" {
		t.Fatal("manifest aliases retained source")
	}
	if paths := prompt.ContentOmittedPaths(map[string]any{"type": "image", "data": "ordinary tool JSON"}); len(paths) != 0 {
		t.Fatal("opaque JSON tool result was treated as a block")
	}
	r = StandardRequest{Messages: []any{map[string]any{"role": " ASSISTANT ", "reasoning_content": map[string]any{"type": "thinking", "thinking": "body", "signature": "PRIVATE"}}}}
	if got := r.ContextManifest().SourceMessages[0].OmittedPaths; !reflect.DeepEqual(got, []string{"/reasoning_content/signature"}) {
		t.Fatalf("object reasoning has an invalid source pointer: %v", got)
	}
}
