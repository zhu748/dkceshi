package chat

import (
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
	"ds2api/internal/usagestats"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestVercelUsageMatchesClientHistoryAndLedger(t *testing.T) {
	stats := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer stats.Close()
	history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{Usage: stats, ChatHistory: history}
	var req map[string]any
	if err := json.Unmarshal([]byte(`{"usage_accounting_version":1,"finish_reason":"stop","content":"answer","upstream_usage":{"messages":[{"message_id":2,"input_tokens":11,"output_tokens":29,"reasoning_tokens":9,"accumulated_tokens":2551}]}}`), &req); err != nil {
		t.Fatal(err)
	}
	standard := promptcompat.StandardRequest{ResponseModel: "model", ResolvedModel: "model", PromptTokenText: "task", RefFileTokens: 1000}
	usage := finalizeVercelUsage(standard, req)
	if usage["prompt_tokens"] != 11 || usage["completion_tokens"] != 29 || usage["total_tokens"] != 40 {
		t.Fatalf("upstream usage replaced or files double counted: %+v", usage)
	}
	h.recordVercelStreamCompletion(httptest.NewRequest("POST", "/?__stream_release=1", nil), &auth.RequestAuth{CallerID: "caller"}, standard, req)
	snapshot, err := history.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("missing history: %+v", snapshot)
	}
	u := snapshot.Items[0].Usage
	if *u.PromptTokens != 11 || *u.TotalTokens != 40 || *u.AccumulatedTokens != 2551 {
		t.Fatalf("history diverged: %+v", u)
	}
	entries := stats.Summary()
	if len(entries) != 1 || entries[0].TotalTokens != 40 || entries[0].CompletionTokens != 29 || entries[0].Calls != 1 {
		t.Fatalf("ledger diverged: %+v", entries)
	}
	req["finish_reason"] = "stopped"
	partial := finalizeVercelUsage(standard, req)
	if partial["ds2api"].(*tokenusage.Metadata).Complete {
		t.Fatal("partial usage claims completeness")
	}
}

func TestFailedStreamPreservesUsageWithoutBilling(t *testing.T) {
	stats := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer stats.Close()
	history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	session := startChatHistoryWithUsage(history, stats, httptest.NewRequest("POST", "/v1/chat/completions", nil), &auth.RequestAuth{CallerID: "caller"}, promptcompat.StandardRequest{ResponseModel: "model", FinalPrompt: "task"})
	var observed tokenusage.Snapshot
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"usage":{"output_tokens":29},"accumulated_token_usage":2551}`), &raw); err != nil {
		t.Fatal(err)
	}
	observed.Observe(tokenusage.Parse(raw))
	session.errorWithUpstream(502, "interrupted", "upstream_interrupted", "", "partial answer", observed)
	snapshot, err := history.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Status != "error" {
		t.Fatalf("missing error: %+v", snapshot)
	}
	usage := snapshot.Items[0].Usage
	if usage == nil || *usage.CompletionTokens != 29 || *usage.AccumulatedTokens != 2551 || usage.Complete == nil || *usage.Complete {
		t.Fatalf("partial usage lost: %+v", usage)
	}
	if len(stats.Summary()) != 0 {
		t.Fatal("failed stream was billed as a success")
	}
}
