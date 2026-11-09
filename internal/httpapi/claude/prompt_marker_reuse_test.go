package claude

import (
	"strings"
	"testing"

	"ds2api/internal/config"
)

func TestNativeNormalizationBuildsFinalCallerMarkerOnce(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{}`)
	request := map[string]any{"model": "claude-sonnet-4-6", "system": "SYSTEM_CONTEXT", "messages": []any{map[string]any{"role": "user", "content": "USER_CONTEXT"}}, "tools": []any{map[string]any{"name": "read", "input_schema": map[string]any{"type": "object"}}}, "output_config": map[string]any{"format": map[string]any{"type": "json_object"}}}
	normalized, err := normalizeClaudeRequest(config.LoadStore(), request, "PRIVATE_MARKER")
	if err != nil {
		t.Fatal(err)
	}
	standard := normalized.Standard
	rebuilt, _ := standard.BuildPromptFromMessages(standard.Messages)
	if standard.ToolMarker != "PRIVATE_MARKER" || standard.FinalPrompt != rebuilt || standard.PromptTokenText != rebuilt || !strings.Contains(rebuilt, "<|PRIVATE_MARKER|tool_calls>") || !strings.Contains(rebuilt, "SYSTEM_CONTEXT") || !strings.Contains(rebuilt, "USER_CONTEXT") {
		t.Fatal("native normalization and shared rebuild diverged")
	}
}
