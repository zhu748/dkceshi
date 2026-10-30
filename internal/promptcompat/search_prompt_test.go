package promptcompat

import (
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/config"
)

func TestSearchPromptFollowsResolvedModelWithoutDeclaringClientTools(t *testing.T) {
	store := &fakeRouteConfigReader{aliases: config.DefaultModelAliases()}
	store.aliases["web-research"] = "deepseek-v4.1-flash-search"
	store.aliases["search-in-name-only"] = "deepseek-v4.1-flash"
	models := []struct {
		name   string
		search bool
	}{
		{"deepseek-v4.1-flash", false},
		{"deepseek-v4.1-flash-nothinking", false},
		{"deepseek-v4.1-flash-search", true},
		{"deepseek-v4.1-flash-search-nothinking", true},
		{"web-research", true},
		{"search-in-name-only", false},
	}
	for _, model := range models {
		for _, responses := range []bool{false, true} {
			name := model.name + "/chat"
			if responses {
				name = model.name + "/responses"
			}
			t.Run(name, func(t *testing.T) {
				messages := []any{map[string]any{"role": "user", "content": "Research this topic"}}
				req := map[string]any{"model": model.name, "messages": messages}
				var std StandardRequest
				var err error
				if responses {
					std, err = NormalizeOpenAIResponsesRequest(store, req, "")
				} else {
					std, err = NormalizeOpenAIChatRequest(store, req, "")
				}
				if err != nil {
					t.Fatal(err)
				}
				before := std
				out := ApplySearchPrompt(std)
				if strings.Contains(out.FinalPrompt, "Built-in web search protocol") != model.search {
					t.Fatalf("unexpected search instruction gating for %s", model.name)
				}
				if !model.search && !reflect.DeepEqual(before, out) {
					t.Fatal("non-search request changed")
				}
				if RequestBodyHasTools(req) || len(out.ToolNames) != 0 || out.ToolsRaw != nil || strings.Contains(out.FinalPrompt, "<|EPSE|tool_calls>") {
					t.Fatal("upstream web tools became client tools")
				}
				if !reflect.DeepEqual(out.Messages, before.Messages) || out.PromptTokenText != out.FinalPrompt {
					t.Fatal("search injection changed input messages or lost token accounting")
				}
				if got := out.CompletionPayload("session")["search_enabled"]; got != model.search {
					t.Fatalf("unexpected upstream search flag: %v", got)
				}
			})
		}
	}
}

func TestSearchPromptSurvivesThinkingRebuildAndPreservesClientTools(t *testing.T) {
	req := thinkingInjectionRequest()
	req.Search = true
	req.ToolMarker = "Q7ZK3M"
	req.ToolsRaw = []any{map[string]any{"name": "read_file", "parameters": map[string]any{"type": "object"}}}
	req.ToolNames = []string{"read_file"}
	req.FinalPrompt, _ = req.BuildPromptFromMessages(req.Messages)
	req.PromptTokenText = req.FinalPrompt
	before := ApplySearchPrompt(req)
	out := ApplyThinkingInjection(thinkingInjectionStoreStub{enabled: true}, before)
	out = ApplySearchPrompt(out)
	if strings.Count(out.FinalPrompt, "Built-in web search protocol") != 1 || !strings.Contains(out.FinalPrompt, ThinkingInjectionMarker) {
		t.Fatal("thinking rebuild lost or duplicated search instructions")
	}
	if !reflect.DeepEqual(out.ToolNames, []string{"read_file"}) || !strings.Contains(out.FinalPrompt, "<|Q7ZK3M|tool_calls>") || strings.Contains(out.FinalPrompt, "EPSE") {
		t.Fatal("client tool names or caller-specific marker changed")
	}
	if out.PromptTokenText != out.FinalPrompt || !reflect.DeepEqual(out, ApplySearchPrompt(out)) {
		t.Fatal("search injection is not idempotent or token accounting drifted")
	}
}

func TestSearchPromptPreservesAssistantPrefillAndFileTokenText(t *testing.T) {
	req := StandardRequest{
		Search:          true,
		FinalPrompt:     "[User]:Question[Assistant]:Partial answer",
		PromptTokenText: "uploaded history\n[User]:Question[Assistant]:Partial answer",
	}
	out := ApplySearchPrompt(req)
	if !strings.HasPrefix(out.FinalPrompt, "[System]:Built-in web search protocol") || !strings.HasSuffix(out.FinalPrompt, req.FinalPrompt) {
		t.Fatal("search instructions disturbed the assistant prefill")
	}
	if !strings.HasSuffix(out.PromptTokenText, req.PromptTokenText) || !reflect.DeepEqual(out, ApplySearchPrompt(out)) {
		t.Fatal("file token text was discarded or injection repeated")
	}
}

func TestSearchPromptDoesNotReenableDisabledClientTools(t *testing.T) {
	req := map[string]any{
		"model":    "deepseek-v4.1-flash-search",
		"messages": []any{map[string]any{"role": "user", "content": "Research this topic"}},
		"tools":    []any{map[string]any{"name": "read_file"}},
	}
	DisableToolsForRequest(req)
	std, err := NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	out := ApplySearchPrompt(std)
	if !strings.Contains(out.FinalPrompt, "Built-in web search protocol") || !out.ToolsDisabled || len(out.ToolNames) != 0 || out.ToolsRaw != nil || strings.Contains(out.FinalPrompt, "<|EPSE|tool_calls>") {
		t.Fatal("native search and the client-tools restriction were conflated")
	}
}

func TestSearchPromptPreservesSameNamedClientToolsAndLiteralMarker(t *testing.T) {
	tools := []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "search", "parameters": map[string]any{"type": "object", "required": []any{"query"}}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "open", "parameters": map[string]any{"type": "object", "required": []any{"url"}}}},
	}
	std, err := NormalizeOpenAIChatRequest(nil, map[string]any{
		"model":    "deepseek-v4.1-flash-search",
		"messages": []any{map[string]any{"role": "user", "content": "Explain literal EPSE and Q7ZK3M in this text"}},
		"tools":    tools,
	}, "", "Q7ZK3M")
	if err != nil {
		t.Fatal(err)
	}
	out := ApplySearchPrompt(std)
	if !reflect.DeepEqual(out.ToolsRaw, tools) || !reflect.DeepEqual(out.ToolNames, []string{"search", "open"}) || !reflect.DeepEqual(out.ToolChoice, std.ToolChoice) {
		t.Fatal("native search instructions changed the client tool catalog or policy")
	}
	if !strings.Contains(out.FinalPrompt, "Explain literal EPSE and Q7ZK3M in this text") || strings.Contains(out.FinalPrompt, "client-side EPSE tool_calls") || !strings.Contains(out.FinalPrompt, "client-side Q7ZK3M tool_calls") {
		t.Fatal("marker rendering changed user text or exposed the canonical protocol label in generated instructions")
	}
}

func TestSearchPromptSynchronizesTokenTextWithoutDuplicatingFileContext(t *testing.T) {
	final := prependBuiltinSearchPrompt("rendered conversation", "")
	for _, tokenText := range []string{"", "rendered conversation", "uploaded history\n" + final} {
		out := ApplySearchPrompt(StandardRequest{Search: true, FinalPrompt: final, PromptTokenText: tokenText})
		if strings.Count(out.PromptTokenText, "Built-in web search protocol") != 1 || !reflect.DeepEqual(out, ApplySearchPrompt(out)) {
			t.Fatal("search instruction token accounting is missing or duplicated")
		}
		if strings.HasPrefix(tokenText, "uploaded history") && out.PromptTokenText != tokenText {
			t.Fatal("file context token layout changed")
		}
	}
}
