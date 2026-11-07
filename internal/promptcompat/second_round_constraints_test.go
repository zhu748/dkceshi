package promptcompat

import "testing"

func TestOutputSchemaAliasesCannotOverwriteConstraints(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"path"}}
	for _, nested := range []any{
		map[string]any{"schema": map[string]any{"type": "object"}},
		map[string]any{"schema": nil}, "invalid",
	} {
		if _, err := ApplyOutputFormat(StandardRequest{}, map[string]any{"type": "json_schema", "schema": schema, "json_schema": nested}); err == nil {
			t.Errorf("output schema was overwritten by %v", nested)
		}
	}
	if _, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": "TASK", "text": "invalid"}, ""); err == nil {
		t.Error("invalid text format container was ignored")
	}
	standard, err := ApplyOutputFormat(StandardRequest{}, map[string]any{"type": "json_schema", "schema": schema, "json_schema": map[string]any{"schema": schema}})
	if err != nil || standard.OutputFormat.Validate(`{}`) == nil || standard.OutputFormat.Validate(`{"path":"kept"}`) != nil {
		t.Fatalf("equivalent schema aliases lost constraints: %v", err)
	}
}

func TestMessageControlTypesCannotDisappearDuringNormalization(t *testing.T) {
	for _, field := range []string{"role", "prefix", "is_error"} {
		for _, native := range []bool{false, true} {
			message := map[string]any{"role": "user", "content": "TASK", "parts": []any{map[string]any{"text": "TASK"}}, field: 123}
			var err error
			if native {
				err = ValidateContentContainers([]any{message}, "parts", false)
			} else {
				err = ValidateToolHistory([]any{message})
			}
			if err == nil {
				t.Errorf("%s silently ignored; native=%v", field, native)
			}
		}
	}
	for _, value := range []any{nil, false, true} {
		if err := ValidateMessageMetadata(map[string]any{"role": "assistant", "prefix": value, "is_error": value}); err != nil {
			t.Fatalf("valid optional metadata rejected: %v", err)
		}
	}
	if err := ValidateResponsesRequestContext(map[string]any{"input": []any{map[string]any{"type": "message", "role": 123, "content": "TASK"}}}); err == nil {
		t.Fatal("Responses role was discarded before validation")
	}
}
