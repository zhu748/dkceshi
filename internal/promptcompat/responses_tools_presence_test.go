package promptcompat

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesToolTextRoutesLikeItsNormalizedPrompt(t *testing.T) {
	for _, body := range []map[string]any{
		{"instructions": "Available tools:\nread_file(path)", "input": "TASK"},
		{"instructions": "Available tools:\nread_file(path)", "messages": []any{map[string]any{"role": "user", "content": "TASK"}}},
		{"input": []any{map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": "Available tools: read_file(path)"}}}, map[string]any{"role": "user", "content": "TASK"}}},
		{"input": map[string]any{"role": "system", "text": "Available tools: read_file(path)"}},
	} {
		body["model"] = "deepseek-v4.1-flash"
		for _, disabled := range []bool{false, true} {
			req := shallowCopyMap(body)
			if disabled {
				req["tools"] = []any{}
			}
			std, err := NormalizeOpenAIResponsesRequest(nil, req, "")
			if err != nil {
				t.Fatal(err)
			}
			routed := ApplyCallerFlags(nil, httptest.NewRequest("POST", "/v1/responses", nil), req, "openai_responses")
			if routed != !disabled || routed != std.SystemTextToolsApply() || routed != strings.Contains(std.FinalPrompt, "工具调用格式规范") {
				t.Fatalf("route=%v normalized=%v disabled=%v", routed, std.SystemTextToolsApply(), disabled)
			}
			DisableToolsForRequest(req)
			if RequestBodyHasTools(req, "openai_responses") {
				t.Fatal("force-disabled tools reactivated")
			}
		}
	}
}

func TestTextToolRoutingRespectsProtocolAndSourceRole(t *testing.T) {
	for _, req := range []map[string]any{
		{"input": []any{map[string]any{"role": "user", "content": "Available tools: read_file"}}},
		{"input": []any{map[string]any{"type": "function_call_output", "role": "system", "output": "Available tools: read_file"}}},
		{"instructions": "Do not call tools.", "input": "TASK"},
	} {
		if RequestBodyHasTools(req, "openai_responses") {
			t.Fatal("non-catalog input routed as a tool request")
		}
	}
	for _, surface := range []string{"openai_chat", "anthropic_messages", "google_gemini"} {
		req := map[string]any{"instructions": "Available tools: read_file", "input": []any{map[string]any{"role": "system", "content": "Available tools: read_file"}}}
		if RequestBodyHasTools(req, surface) {
			t.Fatalf("%s inferred tools from an unrelated Responses envelope", surface)
		}
	}
}
