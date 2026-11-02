package completionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
)

func TestRepairDefaultsToResolvedOriginalModelAndKeepsIsolatedPrompt(t *testing.T) {
	for _, model := range []string{"deepseek-v4.1-flash", "deepseek-v4.1-flash-search"} {
		ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200, `data: {"p":"response/content","v":"repaired"}`, "data: [DONE]")}}
		history := newRepairHistoryStore(t)
		original := promptcompat.StandardRequest{ResolvedModel: model, ResponseModel: "claude-compatible-alias", FinalPrompt: "PRIVATE ORIGINAL CONTEXT", RefFileIDs: []string{"private-file"}}
		text, err := NewToolCallRepairInvoker(ds, &auth.RequestAuth{}, history, original)(context.Background(), "isolated repair")
		if err != nil || text != "repaired" {
			t.Fatalf("repair failed: %q %v", text, err)
		}
		entries := repairEntries(t, history)
		record, readErr := history.Get(entries[0].ID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if record.Model != model || ds.payloads[0]["prompt"] != "isolated repair" || ds.payloads[0]["parent_message_id"] != nil {
			t.Fatal("repair ignored original model or inherited context")
		}
		if ds.payloads[0]["thinking_enabled"] != true || len(ds.payloads[0]["ref_file_ids"].([]any)) != 0 {
			t.Fatal("repair lost reasoning or inherited files")
		}
	}
}

func TestCustomRepairAPIUsesConfiguredModelAndRecordsProvider(t *testing.T) {
	const key = "custom-secret-key"
	const bad = `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	const answer = `<tool_calls><invoke name="read"><parameter name="path">a.txt</parameter></invoke></tool_calls>`
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("wrong external endpoint or credential")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests <- payload
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": answer, "reasoning_content": "check the wrapper"}, "finish_reason": "stop"}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	cfg := tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL + "/v1", Model: "custom-reasoner", APIKey: key, TimeoutSeconds: 30, ExtraBody: json.RawMessage(`{"reasoning_effort":"high"}`)}
	ctx := tooldiagnostics.WithRepairSettings(context.Background(), cfg)
	recorder := tooldiagnostics.NewRecorder(store, "chat", false)
	ctx = tooldiagnostics.WithRecorder(ctx, recorder)
	ds := &fakeDeepSeekCaller{}
	history := newRepairHistoryStore(t)
	invoke := NewToolCallRepairInvoker(ds, &auth.RequestAuth{DeepSeekToken: "must-not-leak"}, history, promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash"})
	turn := assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: bad}, assistantturn.BuildOptions{Model: "original-model", ToolCallRepair: invoke, ToolCallRepairCtx: ctx})
	if turn.Error != nil || len(turn.ToolCalls) != 1 {
		t.Fatalf("custom format repair failed: %+v", turn.Error)
	}
	recorder.Finish(200, nil, false)
	if len(ds.payloads) != 0 {
		t.Fatal("custom mode used the native upstream")
	}
	received := <-requests
	if received["model"] != "custom-reasoner" || received["reasoning_effort"] != "high" || received["stream"] != false {
		t.Fatal("custom model/parameters ignored")
	}
	messages := received["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" {
		t.Fatal("repair was not isolated")
	}
	for _, field := range []string{"tools", "ref_file_ids", "parent_message_id"} {
		if _, ok := received[field]; ok {
			t.Fatal("main request context leaked")
		}
	}
	entry, err := store.Get(store.List(tooldiagnostics.Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range entry.Attempts[0].Steps {
		if step.Stage == "repair_request" && step.Provider == "custom_api" && step.Model == "custom-reasoner" {
			if step.HistoryID != repairEntries(t, history)[0].ID {
				t.Fatal("repair archive link missing")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("provider/model missing from diagnostics")
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), key) || strings.Contains(string(raw), "must-not-leak") {
		t.Fatal("credential leaked into diagnostic record")
	}
	archive, err := history.Get(repairEntries(t, history)[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if archive.Model != "custom-reasoner" || archive.ReasoningContent != "check the wrapper" {
		t.Fatal("actual model/reasoning missing from repair archive")
	}
}

func TestCustomRepairFailureScrubsKeyAndHonorsCallerDeadline(t *testing.T) {
	for _, slow := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "deadline"}[slow], func(t *testing.T) {
			const key = "never-log-this-key"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if slow {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(401)
				if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "invalid key " + key}}); err != nil {
					t.Log(err)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if slow {
				var shortCancel context.CancelFunc
				ctx, shortCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer shortCancel()
			}
			ctx = tooldiagnostics.WithRepairSettings(ctx, tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "custom", APIKey: key, TimeoutSeconds: 60})
			_, err := NewToolCallRepairInvoker(&fakeDeepSeekCaller{}, &auth.RequestAuth{}, nil)(ctx, "repair")
			if err == nil || strings.Contains(err.Error(), key) {
				t.Fatal("failure missing or key exposed")
			}
			if slow && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("caller deadline was lost: %v", err)
			}
		})
	}
}
