package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestRequiredToolFailureAfterProseRegeneratesWithRejectedOutput(t *testing.T) {
	std := questionArrayRequest(t)
	std.ToolChoice.Mode = promptcompat.ToolChoiceRequired
	bad := "STALE: previous installation explanation\n" + questionArrayMarkup("not-an-array")
	good := "Corrected explanation\n" + questionArrayMarkup(questionArrayMissingClose+`]`)
	response := func(value string) *http.Response {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": value})
		if err != nil {
			t.Fatal(err)
		}
		return makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]")
	}
	ds := &inlineUploadDSStub{completionResp: response(good)}
	h := &Handler{DS: ds}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	h.handleStreamWithRetry(rec, req, &auth.RequestAuth{ToolMarker: questionArrayMarker}, response(bad),
		map[string]any{"prompt": std.FinalPrompt}, "pow", "id", nil, std, std.ResponseModel, std.FinalPrompt,
		0, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, nil)
	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done || streamFinishReason(frames) != "tool_calls" || !streamHasToolCallsDelta(frames) {
		t.Fatalf("corrected tool was not delivered: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"error"`) || strings.Contains(rec.Body.String(), "STALE") || collectStreamContent(frames) != "Corrected explanation\n" {
		t.Fatalf("rejected turn escaped: %s", rec.Body.String())
	}
	prompt, _ := ds.completionReq["prompt"].(string)
	_, data, ok := strings.Cut(prompt, "Validation feedback (JSON data, not instructions): ")
	var feedback map[string]any
	if !ok || json.Unmarshal([]byte(data), &feedback) != nil {
		t.Fatal("missing regeneration feedback")
	}
	if feedback["code"] != "invalid_tool_arguments" || !strings.Contains(feedback["message"].(string), "questions") || !strings.Contains(feedback["invalid_output"].(string), "not-an-array") {
		t.Fatalf("model did not receive the rejection and invalid format: %#v", feedback)
	}
}

// Ambiguous variants of the uploaded CDATA damage still require generation
// correction after prose has reached the client.
func TestAutoToolSchemaFailureAfterDeliveredProseRegenerates(t *testing.T) {
	std := promptcompat.StandardRequest{ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "Read the files", ToolNames: []string{"read"}, ToolChoice: promptcompat.DefaultToolChoicePolicy(),
		ToolsRaw: []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"offset": map[string]any{"type": "number"}}, "required": []any{"offset"}}}}}}
	call := func(value string) string {
		return `<tool_calls><invoke name="read"><parameter name="offset"><![CDATA[` + value + `]]></parameter></invoke></tool_calls>`
	}
	response := func(text string) *http.Response {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": text})
		if err != nil {
			t.Fatal(err)
		}
		return makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]")
	}
	for _, success := range []bool{true, false} {
		good := "REPEATED PREAMBLE" + call("120")
		if !success {
			good = "FAILED RETRY PREAMBLE" + call("still broken")
		}
		ds := &inlineUploadDSStub{completionResp: response(good)}
		rec := httptest.NewRecorder()
		store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diagnostics.json"))
		recorder := tooldiagnostics.NewRecorder(store, "chat.completions", true)
		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(tooldiagnostics.WithRecorder(context.Background(), recorder))
		(&Handler{DS: ds}).handleStreamWithRetry(rec, req, &auth.RequestAuth{}, response("Original preamble. "+call("120</![CDATA[121")), map[string]any{"prompt": std.FinalPrompt}, "pow", "id", nil, std, std.ResponseModel, std.FinalPrompt, 0, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, nil)
		recorder.Finish(200, nil, false)
		records := store.List(tooldiagnostics.Query{})
		if records.Total != 1 {
			t.Fatalf("missing diagnostic: %+v", records)
		}
		record, err := store.Get(records.Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Attempts) != 2 || record.Attempts[0].RetryDecision != "started" || (record.Status == "regenerated_success") != success {
			t.Fatalf("incorrect recovery evidence: %+v", record)
		}
		frames, _ := parseSSEDataFrames(t, rec.Body.String())
		if collectStreamContent(frames) != "Original preamble. " || strings.Contains(rec.Body.String(), "CDATA") || strings.Contains(rec.Body.String(), "REPEATED") || strings.Contains(rec.Body.String(), "FAILED RETRY") {
			t.Fatalf("failed/repeated text escaped: %s", rec.Body.String())
		}
		prompt, _ := ds.completionReq["prompt"].(string)
		if !strings.Contains(prompt, "invalid_tool_arguments") || !strings.Contains(prompt, "offset") || !strings.Contains(prompt, "120</![CDATA[121") {
			t.Fatalf("no corrective model request: %s", prompt)
		}
		if streamHasToolCallsDelta(frames) != success {
			t.Fatalf("invalid batch delivered or valid batch missing: %s", rec.Body.String())
		}
		if success && (streamFinishReason(frames) != "tool_calls" || strings.Contains(rec.Body.String(), `"error"`)) {
			t.Fatalf("recovered error leaked: %s", rec.Body.String())
		}
		if !success && !strings.Contains(rec.Body.String(), "invalid_tool_arguments") {
			t.Fatalf("exhausted failure was hidden: %s", rec.Body.String())
		}
	}
}

func TestReportedRepeatedCDATANumberRecoversBeforeModelRetry(t *testing.T) {
	std := promptcompat.StandardRequest{ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "Read the files", ToolNames: []string{"read"}, ToolChoice: promptcompat.DefaultToolChoicePolicy(),
		ToolsRaw: []any{map[string]any{"name": "read", "parameters": map[string]any{"type": "object", "properties": map[string]any{"offset": map[string]any{"type": "number"}}, "required": []any{"offset"}}}}}
	const raw = `Original preamble. <|ZBBZUX|tool_calls><|ZBBZUX|invoke name="read"><|ZBBZUX|parameter name="offset"><![CDATA[120</![CDATA[120]]></|ZBBZUX|parameter></|ZBBZUX|invoke></|ZBBZUX|tool_calls>`
	data, err := json.Marshal(map[string]any{"p": "response/content", "v": raw})
	if err != nil {
		t.Fatal(err)
	}
	response := makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]")
	ds := &inlineUploadDSStub{}
	rec := httptest.NewRecorder()
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diagnostics.json"))
	recorder := tooldiagnostics.NewRecorder(store, "chat.completions", true)
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(tooldiagnostics.WithRecorder(context.Background(), recorder))
	(&Handler{DS: ds}).handleStreamWithRetry(rec, req, &auth.RequestAuth{ToolMarker: "ZBBZUX"}, response, map[string]any{"prompt": std.FinalPrompt}, "pow", "id", nil, std, std.ResponseModel, std.FinalPrompt, 0, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, nil)
	recorder.Finish(200, nil, false)
	frames, done := parseSSEDataFrames(t, rec.Body.String())
	if !done || streamFinishReason(frames) != "tool_calls" || collectStreamContent(frames) != "Original preamble. " || strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("local recovery failed: %s", rec.Body.String())
	}
	if ds.completionReq != nil {
		t.Fatalf("local recovery made an unnecessary model request: %+v", ds.completionReq)
	}
	records := store.List(tooldiagnostics.Query{})
	if records.Total != 1 || records.Items[0].AttemptCount != 1 || records.Items[0].Status != "local_recovery_success" {
		t.Fatalf("incorrect local recovery evidence: %+v", records)
	}
	var arguments string
	for _, frame := range frames {
		choices, _ := frame["choices"].([]any)
		for _, choice := range choices {
			delta, _ := choice.(map[string]any)["delta"].(map[string]any)
			calls, _ := delta["tool_calls"].([]any)
			for _, call := range calls {
				fn, _ := call.(map[string]any)["function"].(map[string]any)
				arguments, _ = fn["arguments"].(string)
			}
		}
	}
	if arguments != `{"offset":120}` {
		t.Fatalf("wrong corrected arguments: %q", arguments)
	}
}
