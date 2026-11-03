package claude

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRegressionClaudeToolResultPreservesText(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"long file", strings.Repeat("line of file contents\n", 100) + "END_OF_FILE_SENTINEL"},
		{"unicode", strings.Repeat("中", 400) + "END_OF_FILE_SENTINEL"},
		{"ordinary identifier", strings.Repeat("A", 600)},
		{"exact whitespace", "  first line\n\tsecond line\n  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := normalizeClaudeMessages([]any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c1", "content": []any{map[string]any{"type": "text", "text": tc.body}}}}}})
			raw := messages[0].(map[string]any)["content"].(string)
			var result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			got := result.Content[0].Text
			if got != tc.body {
				t.Fatalf("tool body changed: input=%d bytes output=%d bytes validUTF8=%v tailPreserved=%v", len(tc.body), len(got), utf8.ValidString(got), strings.Contains(got, "END_OF_FILE_SENTINEL"))
			}
		})
	}
}

func TestUnknownClaudeBlockTruncationKeepsUTF8(t *testing.T) {
	block := map[string]any{"type": "unknown", "text": strings.Repeat("中", 400)}
	for _, text := range []string{formatClaudeUnknownBlockForPrompt(block), sanitizeClaudeStringForPrompt("text", block["text"].(string))} {
		if !utf8.ValidString(text) || strings.ContainsRune(text, '\uFFFD') || !strings.HasSuffix(text, "...(truncated)") {
			t.Fatal("unknown block truncation corrupted UTF-8")
		}
	}
}
