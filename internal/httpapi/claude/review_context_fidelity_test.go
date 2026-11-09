package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeWhitespaceThinkingAndReferencedTextDocumentRoundTrip(t *testing.T) {
	raw := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "document", "file_id": "file-existing", "source": map[string]any{"type": "text", "data": "DOCUMENT_BODY"}}}},
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": " \r\n\t"}, map[string]any{"type": "text", "text": "ANSWER"}}},
	}}
	native, err := normalizeClaudeRequest(nil, raw)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := promptcompat.NormalizeOpenAIChatRequest(nil, native.Standard.OpenAIChatRequest(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{native.Standard.FinalPrompt, bridge.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(bridge.Messages)} {
		for _, want := range []string{"DOCUMENT_BODY", "file-existing", "[reasoning_content]\n \r\n\t\n[/reasoning_content]", "ANSWER"} {
			if !strings.Contains(text, want) {
				t.Fatalf("native or internal Chat hop lost %q: %q", want, text)
			}
		}
	}
}
