package contextfile

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestCurrentInputFilePreservesSearchFlagWithoutSearchInjection(t *testing.T) {
	for _, systemTools := range []bool{false, true} {
		name := "structured_client_tools"
		if systemTools {
			name = "system_text_client_tools"
		}
		t.Run(name, func(t *testing.T) {
			req := markerStandardRequest(markerUnderTest)
			req.Search = true
			req.Thinking = true
			req.ThinkingInjectionPrompt = promptcompat.DefaultThinkingInjectionPrompt
			req.ResolvedModel = "deepseek-v4.1-flash-search"
			if systemTools {
				req.ToolsRaw = nil
				req.Messages = append([]any{map[string]any{"role": "system", "content": "Available tools: read_file"}}, req.Messages...)
			}
			uploader := &recordingUploader{}
			out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
			if err != nil {
				t.Fatal(err)
			}
			if !out.CurrentInputFileApplied || !out.Search || out.CompletionPayload("session")["search_enabled"] != true || strings.Contains(out.FinalPrompt, "Built-in web search protocol") || strings.Contains(out.HistoryText, "Built-in web search protocol") {
				t.Fatal("history split lost the native search flag or injected search instructions")
			}
			if !strings.HasPrefix(out.FinalPrompt, "<｜System｜>"+promptcompat.DefaultThinkingInjectionPrompt+"\n\n") || strings.Count(out.FinalPrompt, promptcompat.ThinkingInjectionMarker) != 1 {
				t.Fatal("history split moved or duplicated the leading thinking instruction")
			}
			if strings.Count(out.HistoryText, promptcompat.ThinkingInjectionMarker) != 1 || strings.Count(out.PromptTokenText, promptcompat.ThinkingInjectionMarker) != 2 {
				t.Fatal("token text must count one thinking instruction in the file and one in the live prompt")
			}
			if !strings.Contains(out.PromptTokenText, out.FinalPrompt) || !strings.Contains(out.PromptTokenText, out.HistoryText) {
				t.Fatal("token text must include uploaded history and the live prompt")
			}
			if strings.Contains(out.FinalPrompt, "EPSE") || !strings.Contains(out.FinalPrompt, "<|"+markerUnderTest+"|tool_calls>") {
				t.Fatal("history rebuild lost the caller-specific tool marker")
			}
			if strings.Contains(out.FinalPrompt, "Tool: read_file") {
				t.Fatal("history rebuild copied tool schemas out of TOOLS.txt")
			}
			if !systemTools && (!strings.Contains(out.FinalPrompt, "JSON Schemas for client-provided tools") || strings.Contains(out.FinalPrompt, "Use only those tools")) {
				t.Fatal("TOOLS.txt instructions incorrectly restrict upstream native capabilities")
			}
		})
	}
}
