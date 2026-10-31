package chat

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
)

func TestNodePromptEchoHistoryKeepsErrorClassification(t *testing.T) {
	store := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{ChatHistory: store}
	h.recordVercelStreamCompletion(httptest.NewRequest("POST", "/", nil), &auth.RequestAuth{CallerID: "caller"}, promptcompat.StandardRequest{ResponseModel: "model"}, map[string]any{
		"finish_reason": "error", "error_code": "upstream_prompt_echo", "error": "Upstream repeated generated instructions instead of producing an answer.",
	})
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("expected one error history entry: snapshot=%+v err=%v", snapshot, err)
	}
	entry := snapshot.Items[0]
	if entry.Status != "error" || entry.StatusCode != 502 || entry.FinishReason != "upstream_prompt_echo" {
		t.Fatalf("Node error was mislabeled: %+v", entry)
	}
}
