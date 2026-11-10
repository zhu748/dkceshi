package completionruntime

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func BenchmarkNonStreamLargeContext(b *testing.B) {
	for _, tools := range []bool{false, true} {
		name := "text"
		if tools {
			name = "tool"
		}
		b.Run(name, func(b *testing.B) {
			prompt := strings.Repeat("context with 中文 and source code.\n", 4096)
			request := promptcompat.StandardRequest{
				ResolvedModel: "deepseek-v4.1-flash", ResponseModel: "deepseek-v4.1-flash",
				FinalPrompt: prompt, PromptTokenText: prompt,
			}
			output := "ready"
			if tools {
				request.ToolNames = []string{"read"}
				request.ToolsRaw = []any{map[string]any{"type": "function", "function": map[string]any{
					"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}},
				}}}
				output = `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
			}
			// Warm the tokenizer and schema compiler outside the measurement.
			run := func() {
				response := sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"`+strings.ReplaceAll(output, `"`, `\"`)+`"}`, "data: [DONE]")
				result, err := ExecuteNonStreamStartedWithRetry(context.Background(), &fakeDeepSeekCaller{}, &auth.RequestAuth{}, StartResult{Response: response, Request: request}, Options{})
				if err != nil || (tools && len(result.Turn.ToolCalls) != 1) || (!tools && result.Turn.Text != output) {
					b.Fatalf("invalid result: %+v %+v", result.Turn, err)
				}
			}
			run()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				run()
			}
		})
	}
}
