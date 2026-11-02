package chat

import (
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolContextDoesNotDelayFinalAnswer(t *testing.T) {
	for _, name := range []string{"plain_answer", "structured_catalog_final_answer", "text_catalog_final_answer", "history_final_answer", "actual_tool_call"} {
		t.Run(name, func(t *testing.T) {
			messages := []any{map[string]any{"role": "user", "content": "hello"}}
			req := map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}
			if name == "structured_catalog_final_answer" || name == "actual_tool_call" {
				req["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}
			}
			if name == "text_catalog_final_answer" {
				req["messages"] = append([]any{map[string]any{"role": "system", "content": "Available tools: read"}}, messages...)
			}
			if name == "history_final_answer" {
				req["messages"] = []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c1", "type": "function", "function": map[string]any{"name": "read", "arguments": "{}"}}}}, map[string]any{"role": "tool", "tool_call_id": "c1", "content": "file loaded"}, map[string]any{"role": "user", "content": "summarize"}}
			}
			std, err := promptcompat.NormalizeOpenAIChatRequest(nil, req, "")
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, std.ResponseModel, std.FinalPrompt, true, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, true, true)
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "thinking", Text: "Checking the details before responding."}}})
			raw := "This is the final answer after reading the file. Everything is ready."
			if name == "actual_tool_call" {
				raw = `<tool_calls><invoke name="read"></invoke></tool_calls>`
			}
			for _, part := range []string{raw[:len(raw)/2], raw[len(raw)/2:]} {
				rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: part}}})
			}
			before := rec.Body.String()
			earlyText := strings.Contains(before, `"content":`)
			earlyCalls := strings.Contains(before, `"tool_calls":`)
			thinking := strings.Contains(before, `"reasoning_content":`)
			rt.finalize("stop", false)
			after := rec.Body.String()
			t.Logf("answer_before_finalize=%v tool_call_before_finalize=%v reasoning_before_finalize=%v final_error=%q", earlyText, earlyCalls, thinking, rt.finalErrorCode)
			if earlyText != (name != "actual_tool_call") || earlyCalls || !thinking || rt.finalErrorCode != "" {
				t.Fatalf("unexpected timing: %s", after)
			}
			if name == "actual_tool_call" && !strings.Contains(after, `"tool_calls":`) {
				t.Fatal("missing finalized tool call")
			}
			if name != "actual_tool_call" {
				frames, _ := parseSSEDataFrames(t, after)
				if got := collectStreamContent(frames); got != raw {
					t.Fatalf("answer duplicated or lost: %q", got)
				}
			}
		})
	}
}
