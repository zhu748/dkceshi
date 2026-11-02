package promptcompat

import (
	"strings"
	"testing"
)

func TestRegressionResponsesMergeDropsBody(t *testing.T) {
	call := func(id string) []any {
		return []any{map[string]any{"id": id, "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}}}
	}
	got := NormalizeResponsesInputAsMessages([]any{map[string]any{"role": "assistant", "content": "first explanation", "tool_calls": call("c1")}, map[string]any{"role": "assistant", "content": "second critical explanation", "tool_calls": call("c2")}})
	prompt := RenderPromptText(got, false)
	if !strings.Contains(prompt, "second critical explanation") {
		t.Fatalf("second assistant body lost: %s", prompt)
	}
}
func TestRegressionNegativeToolMention(t *testing.T) {
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "system", "content": "Do not call tools. Answer in plain text."}, map[string]any{"role": "user", "content": "Hello"}}}
	std, err := NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(std.FinalPrompt, "<|EPSE|tool_calls>") {
		t.Fatalf("negative instruction injected tool protocol; routed tools=%v", RequestBodyHasTools(req))
	}
}
