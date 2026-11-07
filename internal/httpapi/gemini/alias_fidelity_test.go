package gemini

import (
	"strings"
	"testing"
)

func TestGeminiSystemPartsAndTextCannotDiscardInstructions(t *testing.T) {
	for _, parts := range [][]any{{}, {map[string]any{"text": "PARTS"}}} {
		req := map[string]any{"systemInstruction": map[string]any{"parts": parts, "text": "DIFFERENT"}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
			t.Fatal("conflicting system instruction accepted")
		}
	}
	for _, instruction := range []map[string]any{{"text": "  RULE\r\n"}, {"parts": []any{map[string]any{"text": "  RULE\r\n"}}, "text": "  RULE\r\n"}} {
		req := map[string]any{"systemInstruction": instruction, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}}
		std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
		if err != nil || strings.Count(std.FinalPrompt, "  RULE\r\n") != 1 {
			t.Fatalf("matching instruction alias lost or repeated: %v %s", err, std.FinalPrompt)
		}
	}
}

func TestGeminiFunctionSchemaAliasesCannotOverwriteEachOther(t *testing.T) {
	for _, equal := range []bool{false, true} {
		schema := map[string]any{"type": "object", "required": []any{"path"}}
		other := any(map[string]any{"type": "object"})
		if equal {
			other = schema
		}
		req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "TASK"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "OBJECT", "required": []any{"path"}}, "parametersJsonSchema": other}}}}}
		_, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
		if (err == nil) != equal {
			t.Fatalf("schema alias equality=%v error=%v", equal, err)
		}
	}
}
