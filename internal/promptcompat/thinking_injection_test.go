package promptcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestThinkingInjectionIsFirstSystemComponent(t *testing.T) {
	for _, messages := range [][]any{
		{map[string]any{"role": "user", "content": "latest"}},
		{map[string]any{"role": "system", "content": "BASE-SYSTEM"}, map[string]any{"role": "developer", "content": "BASE-DEVELOPER"}, map[string]any{"role": "user", "content": "latest"}},
		{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "latest"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/image.png"}}}}},
	} {
		req := thinkingInjectionRequest()
		req.Messages = messages
		before := NormalizeOpenAIContentForPrompt(messages[len(messages)-1].(map[string]any)["content"])
		out := ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true}, req)
		if !strings.HasPrefix(out.FinalPrompt, "<｜System｜>"+DefaultThinkingInjectionPrompt+"\n\n") {
			t.Fatalf("thinking instruction is not the first system component: %q", out.FinalPrompt)
		}
		if !reflect.DeepEqual(out.Messages, messages) || before != NormalizeOpenAIContentForPrompt(messages[len(messages)-1].(map[string]any)["content"]) {
			t.Fatal("thinking injection rewrote client messages")
		}
		if !strings.Contains(out.FinalPrompt, "[User]:"+before) {
			t.Fatalf("user content was lost: %q", out.FinalPrompt)
		}
	}
}

func TestThinkingInjectionUsesCustomSystemPrefix(t *testing.T) {
	out := ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true, prompt: "custom thinking format"}, thinkingInjectionRequest())
	if !strings.HasPrefix(out.FinalPrompt, "<｜System｜>custom thinking format\n\n") {
		t.Fatalf("custom instruction was not placed first: %q", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, ThinkingInjectionMarker) {
		t.Fatal("default thinking instruction was added with a custom prompt")
	}
}

func TestThinkingInjectionUserQuoteDoesNotSuppressSystemPrefix(t *testing.T) {
	req := thinkingInjectionRequest()
	req.Messages = []any{map[string]any{"role": "user", "content": "Explain this prompt:\n" + DefaultThinkingInjectionPrompt}}
	out := ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true}, req)
	if !strings.HasPrefix(out.FinalPrompt, "<｜System｜>"+DefaultThinkingInjectionPrompt) || !reflect.DeepEqual(out.Messages, req.Messages) {
		t.Fatal("a user quotation suppressed the generated system instruction or was modified")
	}
}
