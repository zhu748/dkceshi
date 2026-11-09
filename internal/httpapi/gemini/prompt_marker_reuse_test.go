package gemini

import (
	"strings"
	"testing"
)

func TestNativeNormalizationBuildsFinalCallerMarkerOnce(t *testing.T) {
	request := map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "SYSTEM_CONTEXT"}}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "USER_CONTEXT"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "OBJECT"}}}}}, "generationConfig": map[string]any{"responseMimeType": "application/json"}}
	standard, err := normalizeGeminiRequest(testGeminiConfig{}, "gemini-2.5-pro", request, false, "PRIVATE_MARKER")
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _ := standard.BuildPromptFromMessages(standard.Messages)
	if standard.ToolMarker != "PRIVATE_MARKER" || standard.FinalPrompt != rebuilt || standard.PromptTokenText != rebuilt || !strings.Contains(rebuilt, "<|PRIVATE_MARKER|tool_calls>") || !strings.Contains(rebuilt, "SYSTEM_CONTEXT") || !strings.Contains(rebuilt, "USER_CONTEXT") {
		t.Fatal("native normalization and shared rebuild diverged")
	}
}
