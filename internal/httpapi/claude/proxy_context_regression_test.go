package claude

import (
	"ds2api/internal/promptcompat"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeProxyKeepsToolPolicyAndErrorResult(t *testing.T) {
	for _, choice := range []string{`{"type":"none"}`, `{"type":"auto","disable_parallel_tool_use":true}`, `{"type":"tool","name":"check"}`} {
		capture := &openAIProxyCaptureStub{}
		h := &Handler{OpenAI: capture}
		body := `{"model":"deepseek-v4.1-flash","tools":[{"name":"check","input_schema":{"type":"object"}}],"tool_choice":` + choice + `,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"check","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","is_error":true,"content":"FAILED"}]}]}`
		rec := httptest.NewRecorder()
		h.Messages(rec, httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages?__stream_prepare=1", strings.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("proxy failed: %d %s", rec.Code, rec.Body.String())
		}
		std, err := promptcompat.NormalizeOpenAIChatRequest(nil, capture.seenReq, "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(choice, `"none"`) && !std.ToolChoice.IsNone() {
			t.Fatal("none became auto")
		}
		if strings.Contains(choice, "disable_parallel") && !std.ToolChoice.DisableParallel {
			t.Fatal("parallel restriction lost")
		}
		if strings.Contains(choice, `"tool"`) && std.ToolChoice.ForcedName != "check" {
			t.Fatal("forced tool lost")
		}
		result := std.Messages[len(std.Messages)-1].(map[string]any)
		if result["is_error"] != true || result["tool_call_id"] != "a" {
			t.Fatalf("result metadata lost: %#v", result)
		}
	}
}
