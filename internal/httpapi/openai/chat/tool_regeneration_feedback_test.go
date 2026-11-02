package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
)

func TestToolSchemaFailureAfterProseRegeneratesWithRejectedOutput(t *testing.T) {
	std := questionArrayRequest(t)
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
