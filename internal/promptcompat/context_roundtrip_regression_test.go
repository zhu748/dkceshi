package promptcompat

import (
	"ds2api/internal/toolcall"
	"strings"
	"testing"
)

func TestTextToolHistoryPreservesLiteralTypesAndWhitespace(t *testing.T) {
	for _, literal := range []string{`true`, `null`, `{"a":1}`, `[1,2]`, `  "quoted"  `} {
		raw := `<tool_calls><invoke name="check" call_id="a"><parameter name="mode"><![CDATA[` + literal + `]]></parameter></invoke></tool_calls>`
		history := BuildOpenAIHistoryTranscript([]any{map[string]any{"role": "assistant", "content": raw}})
		parsed := toolcall.ParseToolCalls(history, nil)
		calls := toolcall.NormalizeParsedToolCallsForSchemas(parsed, nil)
		if len(calls) != 1 || calls[0].Input["mode"] != literal || calls[0].CallID != "a" {
			t.Fatalf("history changed literal %q: %#v", literal, calls)
		}
	}
}

func TestRefusalAndPrefillKeepAssistantRoleInAllTranscripts(t *testing.T) {
	message := map[string]any{"role": "assistant", "content": nil, "refusal": "  REFUSAL BODY  ", "id": "m1"}
	inputs := [][]any{{message}, NormalizeResponsesInputAsMessages([]any{message})}
	for _, messages := range inputs {
		normalized := NormalizeOpenAIMessagesForPrompt(messages, "")
		if len(normalized) != 1 || normalized[0]["role"] != "assistant" || !strings.Contains(asString(normalized[0]["content"]), "  REFUSAL BODY  ") {
			t.Fatalf("refusal changed role or disappeared: %#v", normalized)
		}
		if !strings.Contains(BuildOpenAIHistoryTranscript(messages), "REFUSAL BODY") {
			t.Fatal("refusal missing in history file")
		}
	}
	prefill := NormalizeResponsesInputAsMessages([]any{map[string]any{"role": "assistant", "content": "prefix", "prefix": true, "id": "prefill"}})
	text := RenderPromptText(prefill, false)
	if !strings.HasSuffix(text, "[Assistant]:prefix") {
		t.Fatalf("prefill boundary lost: %s", text)
	}
}
