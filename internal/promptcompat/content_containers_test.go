package promptcompat

import "testing"

func TestNativeValidationPreservesUnknownObjectBlocks(t *testing.T) {
	messages := []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "future_block", "text": 42, "payload": "KEEP"}}}}
	if err := ValidateContentContainers(messages, "content", true); err != nil {
		t.Fatalf("unknown contextual object rejected: %v", err)
	}
	if text := RenderPromptText(messages, false); text == "" {
		t.Fatal("unknown contextual object disappeared")
	}
}
