package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageExtensionsSurvivePromptHistoryAndRepeatedNormalization(t *testing.T) {
	for _, role := range []string{"system", "developer", "user", "assistant", "tool"} {
		message := map[string]any{"role": role, "content": "  BODY\r\n", "custom_context": map[string]any{"note": "  CONTEXT\r\n", "large": json.Number("9007199254740993"), "encrypted_content": "BUSINESS_VALUE"}, "metadata": nil, "cache_control": map[string]any{"type": "ephemeral"}, "thought_signature": "PROVIDER_STATE"}
		if role == "assistant" {
			message["prefix"] = true
		}
		messages := []any{message}
		first := NormalizeOpenAIMessagesForPrompt(messages, "")
		second := NormalizeOpenAIMessagesForPrompt([]any{first[0]}, "")
		for _, current := range [][]map[string]any{first, second} {
			for _, text := range []string{
				buildExtensionPrompt(current),
				BuildOpenAIHistoryTranscript([]any{current[0]}),
			} {
				for _, wanted := range []string{`"note":"  CONTEXT\r\n"`, "9007199254740993", "BUSINESS_VALUE", `"metadata":null`, "  BODY\r\n"} {
					if !strings.Contains(text, wanted) {
						t.Fatalf("%s lost extension or body %q: %s", role, wanted, text)
					}
				}
				if strings.Count(text, "CONTEXT") != 1 || strings.Contains(text, "PROVIDER_STATE") || strings.Contains(text, "ephemeral") {
					t.Fatalf("%s duplicated context or serialized transport state: %s", role, text)
				}
			}
			if role == "assistant" && !strings.HasSuffix(buildExtensionPrompt(current), "[Assistant]:  BODY\r\n") {
				t.Fatal("context displaced assistant prefill")
			}
		}
		if message["thought_signature"] != "PROVIDER_STATE" || message["custom_context"].(map[string]any)["note"] != "  CONTEXT\r\n" {
			t.Fatal("rendering mutated the original message")
		}
	}
}

func buildExtensionPrompt(messages []map[string]any) string {
	raw := make([]any, len(messages))
	for i := range messages {
		raw[i] = messages[i]
	}
	text, _ := BuildOpenAIPrompt(raw, nil, "", DefaultToolChoicePolicy(), false, ToolReminderConfig{})
	return text
}

func TestResponsesItemExtensionsKeepTheirOriginalTurn(t *testing.T) {
	input := []any{
		map[string]any{"role": "user", "type": "message", "content": "TASK", "metadata": "USER_CONTEXT"},
		map[string]any{"type": "function_call", "name": "read", "call_id": "c", "arguments": "{}", "custom_context": "CALL_CONTEXT"},
		map[string]any{"type": "function_call_output", "call_id": "c", "output": "RESULT", "custom_context": "RESULT_CONTEXT"},
		map[string]any{"role": "assistant", "type": "message", "content": "PREFIX", "prefix": true, "metadata": "PREFILL_CONTEXT"},
	}
	req, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": input}, "")
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NormalizeOpenAIChatRequest(nil, req.OpenAIChatRequest(nil), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{req.FinalPrompt, bridge.FinalPrompt, BuildOpenAIHistoryTranscript(req.Messages)} {
		for _, want := range []string{"USER_CONTEXT", "CALL_CONTEXT", "RESULT_CONTEXT", "PREFILL_CONTEXT"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("extension lost or duplicated: %s", text)
			}
		}
	}
	if !strings.HasSuffix(bridge.FinalPrompt, "[Assistant]:PREFIX") {
		t.Fatal("bridge displaced prefill")
	}
}

func TestExtensionBinaryOmissionsAreTracedWithoutDeletingBusinessFields(t *testing.T) {
	message := map[string]any{"role": "user", "content": "TASK", "metadata": map[string]any{"image": map[string]any{"type": "image", "data": "BINARY_SENTINEL"}, "encrypted_content": "BUSINESS_TEXT"}}
	r := StandardRequest{Messages: []any{message}}
	text := BuildOpenAIHistoryTranscript(r.Messages)
	if strings.Contains(text, "BINARY_SENTINEL") || !strings.Contains(text, "BUSINESS_TEXT") {
		t.Fatalf("incorrect extension sanitization: %s", text)
	}
	paths := r.ContextManifest().SourceMessages[0].OmittedPaths
	if len(paths) != 1 || paths[0] != "/metadata/image/data" {
		t.Fatalf("missing extension omission path: %+v", paths)
	}
}

func TestNestedToolExamplesReachBothPromptTransportsAndRejectConflicts(t *testing.T) {
	function := map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}, "input_examples": []any{map[string]any{"path": "  EXAMPLE\r\n"}}}
	tool := map[string]any{"type": "function", "function": function}
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "TASK"}}, "tools": []any{tool}}
	std, err := NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := BuildOpenAIToolsContextTranscript(std.ToolsRaw, std.ToolChoice)
	for _, text := range []string{std.FinalPrompt, catalog} {
		if !strings.Contains(text, `"path":"  EXAMPLE\r\n"`) {
			t.Fatalf("nested examples lost: %s", text)
		}
	}
	tool["input_examples"] = function["input_examples"]
	if _, err := NormalizeOpenAIChatRequest(nil, request, ""); err != nil {
		t.Fatal("matching example aliases rejected", err)
	}
	tool["input_examples"] = nil
	if _, err := NormalizeOpenAIChatRequest(nil, request, ""); err == nil {
		t.Fatal("conflicting example aliases silently selected")
	}
}
