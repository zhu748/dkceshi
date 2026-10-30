package responses

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openaifmt "ds2api/internal/format/openai"
	"ds2api/internal/promptcompat"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

func TestCustomToolsKeepRawInputAndStreamIdentity(t *testing.T) {
	const input = "*** Begin Patch\n[Tool]: literal & <tag>\n9007199254740993\n*** End Patch"
	tools := []any{map[string]any{"type": "custom", "name": "apply_patch", "format": map[string]any{"type": "text"}}}
	calls := []toolcall.ParsedToolCall{{Name: "apply_patch", Input: map[string]any{"input": input}}}
	obj := openaifmt.BuildResponseObjectWithToolCalls("response", "model", "prompt", "", "", calls, tools)
	item := obj["output"].([]any)[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["input"] != input || item["arguments"] != nil {
		t.Fatalf("custom output corrupted: %#v", item)
	}
	replay := promptcompat.NormalizeResponsesInputAsMessages(obj["output"])
	if len(replay) != 1 || replay[0].(map[string]any)["role"] != "assistant" {
		t.Fatalf("custom history corrupted: %#v", replay)
	}
	rec := httptest.NewRecorder()
	policy := promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"apply_patch": {}}}
	runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "resp", "model", "prompt", false, false, true, []string{"apply_patch"}, tools, true, true, policy, "", nil, nil)
	markup := `<|EPSE|tool_calls><|EPSE|invoke name="apply_patch"><|EPSE|parameter name="input"><![CDATA[` + input + `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	lines := []string{}
	for _, part := range []string{markup[:35], markup[35:90], markup[90:]} {
		data, _ := json.Marshal(map[string]any{"p": "response/content", "v": part})
		lines = append(lines, "data: "+string(data))
	}
	lines = append(lines, "data: [DONE]")
	(&Handler{}).consumeResponsesStreamAttempt(httptest.NewRequest("POST", "/v1/responses", nil), makeResponsesOpenAISSEHTTPResponse(lines...), runtime, "text", false, false)
	var streamed, completed map[string]any
	var delta strings.Builder
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		switch event["type"] {
		case "response.custom_tool_call_input.delta":
			delta.WriteString(event["delta"].(string))
		case "response.output_item.done":
			streamed, _ = event["item"].(map[string]any)
		case "response.completed":
			response := event["response"].(map[string]any)
			completed = response["output"].([]any)[0].(map[string]any)
		}
	}
	if streamed == nil || completed == nil || streamed["type"] != "custom_tool_call" || streamed["call_id"] != completed["call_id"] || completed["input"] != input || delta.String() != input {
		t.Fatalf("stream/custom mismatch: %s", rec.Body.String())
	}
}

func TestFunctionArgumentDeltasDoNotRepeatAlreadyEmittedPrefix(t *testing.T) {
	rec := httptest.NewRecorder()
	runtime := newResponsesStreamRuntime(rec, http.NewResponseController(rec), true, "resp", "model", "prompt", false, false, true, nil, nil, true, true, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	runtime.emitFunctionCallDeltaEvents([]toolstream.ToolCallDelta{{Index: 0, Name: "lookup", Arguments: `{"id":`}})
	runtime.emitFunctionCallDoneEvents([]toolcall.ParsedToolCall{{Name: "lookup", Input: map[string]any{"id": float64(3)}}})
	var args strings.Builder
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.function_call_arguments.delta" {
			args.WriteString(event["delta"].(string))
		}
	}
	if args.String() != `{"id":3}` {
		t.Fatalf("arguments duplicated: %s", args.String())
	}
}
