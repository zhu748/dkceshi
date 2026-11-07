package gemini

import (
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiOutputConfigAliasesKeepConstraintsAcrossBridge(t *testing.T) {
	for _, container := range []string{"generationConfig", "generation_config"} {
		for _, schemaKey := range []string{"responseJsonSchema", "response_json_schema", "responseSchema", "response_schema"} {
			schema := map[string]any{"type": "object", "required": []any{"path"}}
			if schemaKey == "responseSchema" || schemaKey == "response_schema" {
				schema["type"] = "OBJECT"
			}
			req := map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "TASK"}}}}, container: map[string]any{schemaKey: schema, "response_mime_type": "application/json"}}
			standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
			if err != nil {
				t.Fatal(err)
			}
			chat, err := promptcompat.NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(req), "")
			if err != nil || !chat.OutputFormat.Enabled() || chat.OutputFormat.Validate(`{}`) == nil || chat.OutputFormat.Validate(`{"path":"kept"}`) != nil {
				t.Fatalf("%s.%s output constraint lost: %v", container, schemaKey, err)
			}
		}
	}
	standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{
		"contents":          []any{map[string]any{"parts": []any{map[string]any{"text": "TASK"}}}},
		"generationConfig":  map[string]any{"responseSchema": map[string]any{"type": "OBJECT"}, "responseMimeType": "application/json"},
		"generation_config": map[string]any{"response_json_schema": map[string]any{"type": "object"}, "response_mime_type": "application/json"},
	}, false)
	if err != nil || !standard.OutputFormat.Enabled() {
		t.Fatalf("equivalent output configs rejected: %v", err)
	}
}

func TestGeminiOutputConfigsRejectMalformedAndConflictingAliases(t *testing.T) {
	for _, cfg := range []map[string]any{
		{"generationConfig": "invalid"},
		{"generationConfig": map[string]any{"responseMimeType": 123}},
		{"generationConfig": map[string]any{"responseMimeType": "application/json", "response_mime_type": "text/plain"}},
		{"generationConfig": map[string]any{"responseJsonSchema": map[string]any{}, "response_json_schema": nil}},
		{"generationConfig": map[string]any{"responseMimeType": "application/json"}, "generation_config": map[string]any{"response_mime_type": "text/plain"}},
	} {
		cfg["contents"] = []any{map[string]any{"parts": []any{map[string]any{"text": "TASK"}}}}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", cfg, false); err == nil {
			t.Fatalf("output policy disappeared: %+v", cfg)
		}
	}
	for _, message := range []map[string]any{
		{"role": 123, "parts": []any{map[string]any{"text": "TASK"}}},
		{"role": "model", "prefix": "true", "parts": []any{map[string]any{"text": "TASK"}}},
		{"parts": []any{map[string]any{"thought": "true", "text": "THOUGHT"}}},
		{"parts": []any{map[string]any{"functionResponse": map[string]any{"name": 123, "response": "RESULT"}}}},
	} {
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{"contents": []any{message}}, false); err == nil {
			t.Fatalf("invalid native metadata disappeared: %+v", message)
		}
	}
}
