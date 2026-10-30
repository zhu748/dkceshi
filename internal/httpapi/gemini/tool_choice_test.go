package gemini

import "testing"

func TestRegressionGeminiFunctionCallingConfig(t *testing.T) {
	for _, mode := range []string{"NONE", "ANY"} {
		t.Run(mode, func(t *testing.T) {
			req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object"}}}}}, "toolConfig": map[string]any{"functionCallingConfig": map[string]any{"mode": mode, "allowedFunctionNames": []any{"lookup"}}}}
			got, err := normalizeGeminiRequest(testGeminiConfig{}, "gemini-2.5-pro", req, false)
			if err != nil {
				t.Fatal(err)
			}
			if (mode == "NONE" && !got.ToolChoice.IsNone()) || (mode == "ANY" && !got.ToolChoice.IsRequired()) {
				t.Fatalf("functionCallingConfig ignored: %#v", got.ToolChoice)
			}
		})
	}
}

func TestGeminiUppercaseSchemaAndJSONSchemaVariant(t *testing.T) {
	req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "lookup"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "lookup", "parameters": map[string]any{"type": "OBJECT", "properties": map[string]any{"path": map[string]any{"type": "STRING", "nullable": true}}, "required": []any{"path"}}}}}}}
	if _, err := normalizeGeminiRequest(testGeminiConfig{}, "gemini-2.5-pro", req, false); err != nil {
		t.Fatal(err)
	}
}
