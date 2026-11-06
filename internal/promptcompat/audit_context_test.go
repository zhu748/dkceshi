package promptcompat

import (
	"strings"
	"testing"
)

func TestMalformedToolHistoryCannotDisappearDuringNormalization(t *testing.T) {
	for _, messages := range [][]any{
		{42},
		{map[string]any{"role": "assistant", "tool_calls": "broken"}},
		{map[string]any{"role": "assistant", "function_call": map[string]any{"arguments": "LOST"}}},
		{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c1", "function": map[string]any{"arguments": "LOST"}}}}},
	} {
		for _, normalize := range []func(map[string]any) error{
			func(req map[string]any) error { _, err := NormalizeOpenAIChatRequest(nil, req, ""); return err },
			func(req map[string]any) error { _, err := NormalizeOpenAIResponsesRequest(nil, req, ""); return err },
		} {
			if err := normalize(map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}); err == nil {
				t.Fatalf("malformed history accepted: %#v", messages)
			}
		}
	}
}

func TestToolExamplesReachLivePromptAndExternalCatalog(t *testing.T) {
	example := "  EXAMPLE_SENTINEL\r\n"
	tools := []any{map[string]any{"name": "check", "input_schema": map[string]any{"type": "object"}, "input_examples": []any{map[string]any{"key": example}}}}
	messages := []any{map[string]any{"role": "user", "content": "use tool"}}
	live, _ := BuildOpenAIPrompt(messages, tools, "", DefaultToolChoicePolicy(), false, ToolReminderConfig{})
	catalog, _ := BuildOpenAIToolsContextTranscript(tools, DefaultToolChoicePolicy())
	for _, text := range []string{live, catalog} {
		if !strings.Contains(text, `"key":"  EXAMPLE_SENTINEL\r\n"`) {
			t.Fatalf("tool examples missing or changed: %s", text)
		}
	}
}

func TestContextFieldsExposeUnrenderedDataWithoutStoringValues(t *testing.T) {
	r := StandardRequest{Messages: []any{map[string]any{"role": "user", "content": "visible", "custom_context": "PRIVATE_SENTINEL"}}}
	manifest := r.ContextManifest()
	fields := manifest.SourceMessages[0].Fields
	if len(fields) != 3 || fields[1].Name != "custom_context" || fields[1].Disposition != "unrendered" || fields[1].SHA256 != digestContext([]byte(`"PRIVATE_SENTINEL"`)).SHA256 {
		t.Fatalf("missing field coverage: %+v", fields)
	}
	r.SourceContext = manifest.SourceMessages
	copy := r.ContextManifest()
	copy.SourceMessages[0].Fields[0].Name = "mutated"
	if r.SourceContext[0].Fields[0].Name == "mutated" {
		t.Fatal("field coverage aliases source history")
	}
}
