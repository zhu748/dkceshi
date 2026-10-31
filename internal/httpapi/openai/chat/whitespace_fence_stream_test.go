package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClosedFenceBeforeEightCallsDoesNotLeakProtocolText(t *testing.T) {
	tools := []any{map[string]any{"name": "edit", "parameters": map[string]any{
		"type": "object", "required": []any{"file_path", "old_string", "new_string"},
		"properties": map[string]any{"file_path": map[string]any{"type": "string"}, "old_string": map[string]any{"type": "string"}, "new_string": map[string]any{"type": "string"}},
	}}}
	rec := httptest.NewRecorder()
	runtime := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1, "model", "prompt", false, false, true, []string{"edit"}, tools, promptcompat.DefaultToolChoicePolicy(), true, true)
	runtime.toolSieve.Marker = "ZBBZUX"
	chunks := []string{"Evidence:", "\n\n", "```text\n", "sample\n", "```", "\n\n", "Applying changes:\n"}
	wantContent := strings.Join(chunks, "")
	var block strings.Builder
	block.WriteString("<|ZBBZUX|tool_calls>")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&block, `<|ZBBZUX|invoke name="edit"><|ZBBZUX|parameter name="file_path"><![CDATA[file%d.py]]></|ZBBZUX|parameter><|ZBBZUX|parameter name="old_string"><![CDATA[old]]></|ZBBZUX|parameter><|ZBBZUX|parameter name="new_string"><![CDATA[new
<|ZBBZUX|literal>]]></|ZBBZUX|parameter></|ZBBZUX|invoke>`, i)
	}
	block.WriteString("</|ZBBZUX|tool_calls>")
	chunks = append(chunks, block.String())
	var lines []string
	for _, chunk := range chunks {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": chunk})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, "data: "+string(data))
	}
	resp := makeOpenAISSEHTTPResponse(append(lines, "data: [DONE]")...)
	result := (&Handler{}).consumeChatStreamAttempt(httptest.NewRequest(http.MethodPost, "/", nil), resp, runtime, "text", false, nil, true)
	frames, done := parseSSEDataFrames(t, rec.Body.String())
	var content strings.Builder
	calls, finishes := 0, 0
	for _, frame := range frames {
		if frame["error"] != nil {
			t.Fatalf("unexpected stream error: %s", rec.Body.String())
		}
		choices, _ := frame["choices"].([]any)
		for _, raw := range choices {
			choice := raw.(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			text, _ := delta["content"].(string)
			content.WriteString(text)
			wire, _ := delta["tool_calls"].([]any)
			for _, rawCall := range wire {
				call := rawCall.(map[string]any)
				function := call["function"].(map[string]any)
				var args map[string]any
				if json.Unmarshal([]byte(function["arguments"].(string)), &args) != nil || args["file_path"] != fmt.Sprintf("file%d.py", calls) || args["new_string"] != "new\n<|ZBBZUX|literal>" {
					t.Fatalf("call arguments were changed: %#v", call)
				}
				calls++
			}
			if choice["finish_reason"] == "tool_calls" {
				finishes++
			}
		}
	}
	if !result.Terminal || !done || calls != 8 || finishes != 1 || content.String() != wantContent {
		t.Fatalf("protocol leaked or calls duplicated: calls=%d finishes=%d content=%q", calls, finishes, content.String())
	}
}
