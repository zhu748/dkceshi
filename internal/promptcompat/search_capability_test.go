package promptcompat

import (
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/config"
)

func TestSearchCapabilityDoesNotInjectPrompt(t *testing.T) {
	store := &fakeRouteConfigReader{aliases: config.DefaultModelAliases()}
	store.aliases["web-research"] = "deepseek-v4.1-flash-search"
	store.aliases["search-in-name-only"] = "deepseek-v4.1-flash"
	for _, model := range []struct {
		name   string
		search bool
	}{
		{"deepseek-v4.1-flash", false},
		{"deepseek-v4.1-flash-nothinking", false},
		{"deepseek-v4.1-flash-search", true},
		{"deepseek-v4.1-flash-search-nothinking", true},
		{"web-research", true},
		{"search-in-name-only", false},
	} {
		for _, responses := range []bool{false, true} {
			name := model.name + "/chat"
			if responses {
				name = model.name + "/responses"
			}
			t.Run(name, func(t *testing.T) {
				req := map[string]any{"model": model.name, "messages": []any{map[string]any{"role": "user", "content": "Research this topic"}}}
				var out StandardRequest
				var err error
				if responses {
					out, err = NormalizeOpenAIResponsesRequest(store, req, "")
				} else {
					out, err = NormalizeOpenAIChatRequest(store, req, "")
				}
				if err != nil {
					t.Fatal(err)
				}
				if out.Search != model.search || out.CompletionPayload("session")["search_enabled"] != model.search {
					t.Fatal("native search capability was lost")
				}
				if strings.Contains(out.FinalPrompt, "Built-in web search protocol") || out.ToolsRaw != nil || len(out.ToolNames) != 0 || out.PromptTokenText != out.FinalPrompt {
					t.Fatal("search model injected a prompt or client tools")
				}
				out.Search = !out.Search
				if rebuilt, _ := out.BuildPromptFromMessages(out.Messages); rebuilt != out.FinalPrompt {
					t.Fatal("search capability changed the rendered prompt")
				}
			})
		}
	}
}

func TestSearchCapabilityPreservesClientToolsAndThinking(t *testing.T) {
	tools := []any{
		map[string]any{"name": "search", "parameters": map[string]any{"type": "object"}},
		map[string]any{"name": "open", "parameters": map[string]any{"type": "object"}},
		map[string]any{"name": "web_search", "parameters": map[string]any{"type": "object"}},
	}
	std, err := NormalizeOpenAIChatRequest(nil, map[string]any{
		"model": "deepseek-v4.1-flash-search", "tools": tools,
		"messages": []any{map[string]any{"role": "user", "content": "Keep literal EPSE and Q7ZK3M"}},
	}, "", "Q7ZK3M")
	if err != nil {
		t.Fatal(err)
	}
	out := ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true}, std)
	if !reflect.DeepEqual(out.ToolsRaw, tools) || !reflect.DeepEqual(out.ToolNames, []string{"search", "open", "web_search"}) || !reflect.DeepEqual(out.ToolChoice, std.ToolChoice) {
		t.Fatal("search capability altered client tool definitions or policy")
	}
	if !strings.HasPrefix(out.FinalPrompt, thinkingSystemPrefix(DefaultThinkingInjectionPrompt)) || strings.Contains(out.FinalPrompt, "Built-in web search protocol") || !strings.Contains(out.FinalPrompt, "<|Q7ZK3M|tool_calls>") || !strings.Contains(out.FinalPrompt, "Keep literal EPSE and Q7ZK3M") {
		t.Fatal("thinking or caller marker changed, or search prompt was injected")
	}
	if out.PromptTokenText != out.FinalPrompt || !reflect.DeepEqual(out, ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true}, out)) {
		t.Fatal("thinking injection lost token accounting or idempotence")
	}
}

func TestSearchCapabilityPreservesCallerInstructions(t *testing.T) {
	const literal = `Built-in web search protocol: caller-owned instruction with {"queries":"a||b"}`
	out, err := NormalizeOpenAIChatRequest(nil, map[string]any{
		"model":    "deepseek-v4.1-flash-search",
		"messages": []any{map[string]any{"role": "system", "content": literal}, map[string]any{"role": "user", "content": "hello"}},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.FinalPrompt, literal) != 1 {
		t.Fatal("removing server injection must not strip client instructions")
	}
}
