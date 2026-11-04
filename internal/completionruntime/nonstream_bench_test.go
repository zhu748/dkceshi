package completionruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func BenchmarkNonStreamFinalize(b *testing.B) {
	var source strings.Builder
	for i := range 512 {
		fmt.Fprintf(&source, "Entry %04d: preserve identifiers, whitespace, and the original source while validating the final response.\n", i)
	}
	for _, tools := range []bool{false, true} {
		b.Run(fmt.Sprintf("tools=%v", tools), func(b *testing.B) {
			text := source.String()
			req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "Please return the result."}
			if tools {
				text = `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + text + `]]></parameter></invoke></tool_calls>`
				req.ToolNames = []string{"write"}
				req.ToolsRaw = []any{map[string]any{"type": "function", "function": map[string]any{"name": "write", "parameters": map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}}}}}}
			}
			frame, err := json.Marshal(map[string]any{"p": "response/content", "v": text})
			if err != nil {
				b.Fatal(err)
			}
			wire := "data: " + string(frame) + "\n\ndata: [DONE]\n\n"
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			run := func() {
				ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(http.StatusOK, wire)}}
				if _, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, req, Options{}); outErr != nil {
					b.Fatal(outErr)
				}
			}
			run() // Warm tokenizers before measuring the steady request path.
			b.ResetTimer()
			for b.Loop() {
				run()
			}
		})
	}
}
