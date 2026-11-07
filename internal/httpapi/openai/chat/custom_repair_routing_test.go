package chat

import (
	"ds2api/internal/auth"
	diagnosticapi "ds2api/internal/httpapi/tooldiagnostics"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedCustomRepairRoutingAndCorrectiveGeneration(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "repair_success", true: "repair_failure_then_main_regeneration"}[reject], func(t *testing.T) {
			const good = `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
			const bad = `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["model"] != "review-custom-model" || payload["stream"] != false || r.URL.Path != "/v1/chat/completions" {
					t.Error("custom routing lost")
				}
				w.Header().Set("Content-Type", "application/json")
				if reject {
					w.WriteHeader(401)
					if _, err := w.Write([]byte(`{"error":{"message":"test unauthorized"}}`)); err != nil {
						t.Error(err)
					}
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": good}}}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			if err := store.SetRepairSettings(tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL + "/v1", Model: "review-custom-model", TimeoutSeconds: 10}); err != nil {
				t.Fatal(err)
			}
			response := func(raw string) *http.Response {
				data, err := json.Marshal(map[string]any{"p": "response/content", "v": raw})
				if err != nil {
					t.Fatal(err)
				}
				return makeOpenAISSEHTTPResponse("data: "+string(data), "data: [DONE]")
			}
			ds := &inlineUploadDSStub{completionResp: response(good)}
			std := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", ResponseModel: "original-model", Stream: true, ToolNames: []string{"read"}, FinalPrompt: "prompt"}
			handler := diagnosticapi.Middleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h := &Handler{DS: ds}
				h.handleStreamWithRetry(w, r, &auth.RequestAuth{}, response(bad), std.CompletionPayload("session"), "pow", "id", nil, std, std.ResponseModel, "prompt", 0, false, false, std.ToolNames, nil, std.ToolChoice, nil)
			}))
			writer := httptest.NewRecorder()
			handler.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			if requests != 1 || !strings.Contains(writer.Body.String(), `"tool_calls":[`) {
				t.Fatalf("requests=%d output=%s", requests, writer.Body.String())
			}
			if reject && !strings.Contains(ds.completionReq["prompt"].(string), "Validation feedback") {
				t.Fatal("missing main-model corrective generation")
			}
			if !reject && ds.completionReq != nil {
				t.Fatal("custom success called native upstream")
			}
			record, err := store.Get(store.List(tooldiagnostics.Query{}).Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("custom requests=%d main model regenerated=%t status=%s attempts=%d", requests, ds.completionReq != nil, record.Status, len(record.Attempts))
		})
	}
}
