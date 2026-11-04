package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLegacyDefinitionsNormalizeWithoutChangingCaller(t *testing.T) {
	raw := map[string]any{
		"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "read it"}},
		"functions":     []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}},
		"function_call": map[string]any{"name": "read"},
	}
	before, _ := json.Marshal(raw)
	if !RequestBodyHasTools(raw) {
		t.Fatal("legacy definitions did not select the tools route")
	}
	got, err := NormalizeOpenAIChatRequest(nil, raw, "")
	if err != nil || got.ToolChoice.Mode != ToolChoiceForced || got.ToolChoice.ForcedName != "read" || !strings.Contains(got.FinalPrompt, "Tool: read") {
		t.Fatalf("legacy definitions lost: err=%v request=%+v", err, got)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("request was mutated")
	}
	raw["tools"] = []any{}
	if _, err := NormalizeOpenAIChatRequest(nil, raw, ""); err == nil {
		t.Fatal("conflicting legacy and modern catalogs accepted")
	}
}

func TestOutputFormatSurvivesPromptRebuildAndExternalTools(t *testing.T) {
	for _, responses := range []bool{false, true} {
		raw := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "answer"}}}
		format := map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object", "required": []any{"result"}}}
		var got StandardRequest
		var err error
		if responses {
			raw["text"] = map[string]any{"format": format}
			got, err = NormalizeOpenAIResponsesRequest(nil, raw, "")
		} else {
			raw["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "schema": format["schema"]}}
			got, err = NormalizeOpenAIChatRequest(nil, raw, "")
		}
		if err != nil {
			t.Fatal(err)
		}
		rebuilt, _ := got.BuildPromptFromMessages(got.Messages)
		external, _ := got.BuildPromptWithExternalTools(got.Messages, false)
		for _, text := range []string{got.FinalPrompt, rebuilt, external} {
			if strings.Count(text, "Response format:") != 1 || !strings.Contains(text, `"required":["result"]`) || !strings.Contains(text, "answer") {
				t.Fatalf("output constraint lost or duplicated: %s", text)
			}
		}
		if got.OutputFormat.Validate(`{"result":1}`) != nil || got.OutputFormat.Validate(`{}`) == nil {
			t.Fatal("schema constraint not enforced")
		}
	}
}

func TestUnknownAssistantRefusalRemainsInHistory(t *testing.T) {
	text := RenderPromptText([]any{map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "refusal", "refusal": "cannot comply"}}}}, false)
	if !strings.Contains(text, "cannot comply") || !strings.Contains(text, "[Assistant]:") {
		t.Fatal("refusal-only assistant history disappeared")
	}
}
