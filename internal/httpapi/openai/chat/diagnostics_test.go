package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestStreamingDiagnosticPreservesRejectedAttemptAfterRegeneration(t *testing.T) {
	std := questionArrayRequest(t)
	std.ToolChoice.Mode = promptcompat.ToolChoiceRequired
	bad := "rejected prose\n" + questionArrayMarkup("not-an-array")
	good := questionArrayMarkup(questionArrayMissingClose + `]`)
	response := func(value string) *http.Response {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": value})
		if err != nil {
			t.Fatal(err)
		}
		return makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]")
	}
	ds := &inlineUploadDSStub{completionResp: response(good)}
	h := &Handler{DS: ds}
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	handler := diagnosticapi.Middleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.handleStreamWithRetry(w, r, &auth.RequestAuth{ToolMarker: questionArrayMarker, AccountID: "first"}, response(bad), map[string]any{"prompt": std.FinalPrompt}, "pow", "id", nil, std, std.ResponseModel, std.FinalPrompt, 0, false, false, std.ToolNames, std.ToolsRaw, std.ToolChoice, nil)
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	list := store.List(tooldiagnostics.Query{})
	if list.Total != 1 {
		t.Fatalf("wrong request grouping: %+v", list)
	}
	record, err := store.Get(list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "regenerated_success" || !record.Pinned || len(record.Attempts) != 2 {
		t.Fatalf("failed attempt was overwritten: %+v", record)
	}
	if record.Attempts[0].Raw.Text != bad || record.Attempts[0].ErrorCode != "invalid_tool_arguments" || record.Attempts[0].RetryDecision != "started" {
		t.Fatalf("failed scene missing: %+v", record.Attempts[0])
	}
	if record.Attempts[1].Calls[0].Delivery != "written" {
		t.Fatal("final tool delivery not recorded")
	}
}

func TestVercelDiagnosticGroupsValidationAttemptsAndKeepsPrecision(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "diag-secret")
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	h := &Handler{ToolDiagnostics: store}
	tools := []any{map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}, "required": []any{"id"}}}}
	std := promptcompat.StandardRequest{ResponseModel: "test", ToolsRaw: tools, ToolNames: []string{"lookup"}}
	id := h.holdStreamLease(&auth.RequestAuth{AccountID: "account"}, std, "session")
	h.attachLeaseDiagnostics(id, std)
	for i, content := range []string{`<tool_calls><invoke name="lookup"></invoke></tool_calls>`, `<tool_calls><invoke name="lookup"><parameter name="id">9007199254740993</parameter></invoke></tool_calls>`} {
		data, err := json.Marshal(map[string]any{"lease_id": id, "content": content, "diagnostic_attempt": i + 1, "diagnostic_retry_reason": "missing id"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
		req.Header.Set("X-Ds2-Internal-Token", "diag-secret")
		w := httptest.NewRecorder()
		h.ChatCompletions(w, req)
		want := 200
		if i == 0 {
			want = 422
		}
		if w.Code != want {
			t.Fatalf("validation %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	lease, ok := h.lookupStreamLease(id)
	if !ok {
		t.Fatal("missing lease")
	}
	finishLeaseDiagnostics(httptest.NewRequest(http.MethodPost, "/", nil), lease, map[string]any{"diagnostic_attempt": 2, "finish_reason": "tool_calls"})
	list := store.List(tooldiagnostics.Query{})
	record, err := store.Get(list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(record.Attempts) != 2 || !record.Pinned || record.Status != "regenerated_success" {
		t.Fatalf("Node lifecycle not grouped: %+v", record)
	}
	if !strings.Contains(record.Attempts[1].Calls[0].Arguments.Text, "9007199254740993") {
		t.Fatal("diagnostics lost integer precision")
	}
}
