package prompt

import (
	"strings"
	"testing"
)

func TestConflictingHistoricalJSONArgumentsRemainVerbatim(t *testing.T) {
	literal := `{"path":"original.txt","\u0070ath":"other.txt"}`
	text := FormatToolCallsForPrompt([]any{map[string]any{"id": "c1", "function": map[string]any{"name": "read", "arguments": literal}}})
	if !strings.Contains(text, literal) || !strings.Contains(text, `call_id="c1"`) {
		t.Fatal("historical argument conflict was silently resolved")
	}
}
