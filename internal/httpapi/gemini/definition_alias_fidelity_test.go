package gemini

import (
	"encoding/json"
	"testing"

	"ds2api/internal/toolcall"
)

func TestGeminiDefinitionAliasesRetainTheirValidation(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}}
	for _, key := range []string{"parameters", "parametersJsonSchema", "parameters_json_schema", "input_schema", "inputSchema", "schema"} {
		for _, native := range []bool{false, true} {
			definition := map[string]any{"name": "read", key: schema}
			var catalog any = []any{definition}
			if native {
				catalog = []any{map[string]any{"functionDeclarations": []any{definition}}}
			}
			before, _ := json.Marshal(catalog)
			tools := convertGeminiTools(catalog)
			if err := toolcall.ValidateToolSchemas(tools); err != nil {
				t.Fatalf("alias %s native=%v: %v", key, native, err)
			}
			if toolcall.ValidateToolArguments([]toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{}}}, tools) == nil {
				t.Fatalf("%s native=%v lost required fields", key, native)
			}
			if err := toolcall.ValidateToolArguments([]toolcall.ParsedToolCall{{Name: "read", Input: map[string]any{"path": "a"}}}, tools); err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(catalog)
			if string(before) != string(after) {
				t.Fatal("definition conversion mutated the request")
			}
		}
	}
	for _, alias := range []any{nil, map[string]any{"type": "object"}} {
		tools := convertGeminiTools([]any{map[string]any{"name": "read", "parameters": schema, "parametersJsonSchema": alias, "schema": schema}})
		if toolcall.ValidateToolSchemas(tools) == nil {
			t.Fatal("conflicting schema alias disappeared")
		}
	}
}

func TestGeminiPolicyAliasesMustAgreeBeforeSelection(t *testing.T) {
	tools := []any{map[string]any{"name": "read"}, map[string]any{"name": "write"}}
	choice := func(mode string) map[string]any { return map[string]any{"mode": mode} }
	for _, req := range []map[string]any{
		{"toolConfig": "wrong shape"},
		{"toolConfig": map[string]any{"functionCallingConfig": []any{}}},
		{"toolConfig": map[string]any{"functionCallingConfig": map[string]any{"mode": 1}}},
		{"toolConfig": map[string]any{"functionCallingConfig": choice("NONE")}, "tool_config": map[string]any{"function_calling_config": choice("ANY")}},
		{"toolConfig": map[string]any{"functionCallingConfig": choice("AUTO"), "function_calling_config": choice("NONE")}},
		{"toolConfig": map[string]any{"functionCallingConfig": map[string]any{"allowedFunctionNames": []any{"read"}, "allowed_function_names": []any{"write"}}}},
		{"toolConfig": map[string]any{"functionCallingConfig": map[string]any{"allowedFunctionNames": nil, "allowed_function_names": []any{"write"}}}},
	} {
		if _, err := geminiToolPolicy(req, tools); err == nil {
			t.Fatalf("conflicting or malformed policy accepted: %+v", req)
		}
	}
	req := map[string]any{
		"toolConfig":  map[string]any{"functionCallingConfig": map[string]any{"mode": "AUTO", "allowedFunctionNames": []any{"read", "write"}}},
		"tool_config": map[string]any{"function_calling_config": map[string]any{"mode": "auto", "allowed_function_names": []any{"write", "read"}}},
	}
	if _, err := geminiToolPolicy(req, tools); err != nil {
		t.Fatalf("equivalent policy aliases rejected: %v", err)
	}
}
