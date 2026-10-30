package history

import (
	"context"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestCurrentInputFilePreservesNativeSearchInstructions(t *testing.T) {
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
			req = promptcompat.ApplySearchPrompt(req)
			uploader := &recordingUploader{}
			out, err := (Service{Store: enabledCurrentInputConfig{minChars: 1}, DS: uploader}).ApplyCurrentInputFile(context.Background(), &auth.RequestAuth{}, req)
			if err != nil {
				t.Fatal(err)
			}
			if !out.CurrentInputFileApplied || strings.Count(out.FinalPrompt, "Built-in web search protocol") != 1 {
				t.Fatal("history split dropped or duplicated live search instructions")
			}
			if !strings.HasPrefix(out.FinalPrompt, "<｜System｜>"+promptcompat.DefaultThinkingInjectionPrompt+"\n\n[System]:Built-in web search protocol") || strings.Count(out.PromptTokenText, promptcompat.ThinkingInjectionMarker) != 1 {
				t.Fatal("history split moved or duplicated the leading thinking instruction")
			}
			if !strings.Contains(out.PromptTokenText, out.FinalPrompt) || !strings.Contains(out.PromptTokenText, out.HistoryText) {
				t.Fatal("token text must include uploaded history and live search instructions")
			}
			if strings.Contains(out.FinalPrompt, "EPSE") || !strings.Contains(out.FinalPrompt, "<|"+markerUnderTest+"|tool_calls>") {
				t.Fatal("history rebuild lost the caller-specific tool marker")
			}
			if strings.Contains(out.FinalPrompt, "Tool: read_file") {
				t.Fatal("search injection copied tool schemas out of TOOLS.txt")
			}
		})
	}
}
