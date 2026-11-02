package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestNonStreamDiagnosticsIgnoreIntermediateBuildAndKeepRejectedArguments(t *testing.T) {
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	recorder := tooldiagnostics.NewRecorder(store, "chat.completions", false)
	ctx := tooldiagnostics.WithRecorder(context.Background(), recorder)
	bad := `<tool_calls><invoke name="edit"></invoke></tool_calls>`
	good := `<tool_calls><invoke name="edit"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
	response := func(raw string) *http.Response {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": raw})
		if err != nil {
			t.Fatal(err)
		}
		return sseHTTPResponse(200, "data: "+string(data), "data: [DONE]")
	}
	tools := []any{map[string]any{"type": "function", "name": "edit", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []any{"path"}}}}
	std := promptcompat.StandardRequest{ResponseModel: "test", FinalPrompt: "test", ToolsRaw: tools, ToolNames: []string{"edit"}}
	ds := &fakeDeepSeekCaller{responses: []*http.Response{response(good)}}
	result, err := ExecuteNonStreamStartedWithRetry(ctx, ds, &auth.RequestAuth{AccountID: "account"}, StartResult{Response: response(bad), Request: std, Payload: map[string]any{"prompt": "test"}}, Options{RetryEnabled: true})
	if err != nil || len(result.Turn.ToolCalls) != 1 {
		t.Fatalf("completion failed: %+v %v", result, err)
	}
	recorder.Finish(200, nil, false)
	list := store.List(tooldiagnostics.Query{})
	if list.Total != 1 {
		t.Fatalf("unexpected diagnostic count: %d", list.Total)
	}
	record, readErr := store.Get(list.Items[0].ID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(record.Attempts) != 2 || !record.Pinned || record.Status != "regenerated_success" || record.Attempts[0].Raw.Text != bad {
		t.Fatalf("intermediate/final builds duplicated or replaced attempts: %+v", record)
	}
	if len(ds.payloads) != 1 {
		t.Fatal("diagnostics caused extra upstream requests")
	}
}
