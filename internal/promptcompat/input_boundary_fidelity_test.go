package promptcompat

import (
	"strings"
	"testing"
)

func TestResponsesMessageBodyAliasesRequireAgreement(t *testing.T) {
	for _, role := range []string{"user", "assistant", "system", "developer", "tool"} {
		for _, content := range []any{"BODY_A", nil, []any{map[string]any{"type": "text", "text": "BODY_A"}}} {
			req := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{map[string]any{"type": "message", "role": role, "content": content, "text": "BODY_B"}}}
			if _, err := NormalizeOpenAIResponsesRequest(nil, req, ""); err == nil {
				t.Fatalf("conflicting %s message accepted: %#v", role, content)
			}
		}
	}
	for _, content := range []any{"  BODY\r\n", []any{map[string]any{"type": "text", "text": "  BODY\r\n", "annotations": []any{"ANNOTATION_SENTINEL"}}}} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{map[string]any{"role": "user", "content": content, "text": "  BODY\r\n"}}}
		standard, err := NormalizeOpenAIResponsesRequest(nil, req, "")
		if err != nil || strings.Count(standard.FinalPrompt, "  BODY\r\n") != 1 {
			t.Fatalf("equal aliases changed: %v %s", err, standard.FinalPrompt)
		}
		if _, blocks := content.([]any); blocks && !strings.Contains(standard.FinalPrompt, "ANNOTATION_SENTINEL") {
			t.Fatal("matching text aliases lost content metadata")
		}
	}
}

func TestToolSelectorTypesCannotFallBackToAuto(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "read"}}
	for _, value := range []any{nil, false, 1, []any{}, map[string]any{}} {
		for _, choice := range []any{
			map[string]any{"type": value},
			map[string]any{"type": "auto", "allowed_tools": []any{map[string]any{"type": value, "name": "read"}}},
		} {
			if _, err := ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": choice}); err == nil {
				t.Fatalf("invalid selector type accepted: %#v", choice)
			}
		}
	}
	for _, choice := range []any{nil, map[string]any{}, map[string]any{"type": "auto"}} {
		policy, err := ParseOpenAIToolPolicy(map[string]any{"tools": tools, "tool_choice": choice})
		if err != nil || policy.Mode != ToolChoiceAuto {
			t.Fatalf("valid auto policy changed: %v %+v", err, policy)
		}
	}
}

func TestUnsupportedMediaDoesNotBecomeAnEmptyTask(t *testing.T) {
	for _, kind := range []string{"input_audio", "input_video", "audio", "video"} {
		block := map[string]any{"type": kind, "input_audio": map[string]any{"data": "QUJDRA==", "format": "wav"}}
		messages := []any{map[string]any{"role": "user", "content": []any{block}}}
		if _, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, ""); err == nil {
			t.Fatalf("unsupported %s accepted", kind)
		}
		if _, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": messages}, ""); err == nil {
			t.Fatalf("Responses accepted %s", kind)
		}
		if err := ValidateContentContainers([]any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "content": []any{block}}}}}, "content", true); err == nil {
			t.Fatalf("nested native result accepted %s", kind)
		}
	}
	// Business JSON remains data even if it describes audio.
	messages := []any{map[string]any{"role": "tool", "content": map[string]any{"type": "audio", "data": "BUSINESS_SENTINEL"}}}
	standard, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, "BUSINESS_SENTINEL") {
		t.Fatalf("opaque tool JSON changed: %v %s", err, standard.FinalPrompt)
	}
}
