package gemini

import "testing"

func TestGeminiRejectsNamelessFunctionCallBeforePromptAssembly(t *testing.T) {
	req := map[string]any{"contents": []any{map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"args": map[string]any{"text": "LOST"}}}}}}}
	if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
		t.Fatal("nameless native function call silently disappeared")
	}
}

func TestGeminiMalformedPartsCannotBeSilentlySkipped(t *testing.T) {
	for _, content := range []any{42, map[string]any{"role": "user", "parts": "LOST"}, map[string]any{"role": "user", "parts": []any{42}}, map[string]any{"role": "user", "parts": []any{map[string]any{"text": 42}}}, map[string]any{"role": "model", "parts": []any{map[string]any{"text": "kept", "functionCall": map[string]any{"name": "LOST"}}}}} {
		req := map[string]any{"contents": []any{content, map[string]any{"role": "user", "parts": []any{map[string]any{"text": "valid"}}}}}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
			t.Fatalf("invalid source disappeared: %#v", content)
		}
	}
}

func TestGeminiMalformedSystemCannotDisappear(t *testing.T) {
	for _, system := range []any{42, map[string]any{"parts": []any{42}}, map[string]any{"parts": []any{map[string]any{"functionCall": map[string]any{"name": "LOST"}}}}} {
		req := map[string]any{"systemInstruction": system, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "valid"}}}}}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
			t.Fatal("malformed system was silently omitted")
		}
	}
}

func TestGeminiSystemCannotHideMissingOrMalformedContents(t *testing.T) {
	for _, contents := range []any{nil, []any{}, map[string]any{"parts": []any{map[string]any{"text": "LOST"}}}} {
		req := map[string]any{"systemInstruction": "valid system", "contents": contents}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false); err == nil {
			t.Fatal("system-only normalization hid invalid contents")
		}
	}
}
