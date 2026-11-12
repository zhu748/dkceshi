package chathistory

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestHistoryUsageSurvivesSummaryReloadAndSnapshotMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s := New(path)
	entry, err := s.Start(StartParams{UserInput: "task", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	var usage map[string]any
	if err := json.Unmarshal([]byte(`{"prompt_tokens":11,"completion_tokens":29,"total_tokens":40,"completion_tokens_details":{"reasoning_tokens":9},"ds2api":{"source":"upstream","complete":true,"upstream":{"messages":[{"message_id":2,"accumulated_tokens":2551}]}}}`), &usage); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(entry.ID, UpdateParams{Status: "success", Usage: usage, Completed: true}); err != nil {
		t.Fatal(err)
	}
	// Caller mutation must not corrupt details or their future summaries.
	usage["completion_tokens_details"].(map[string]any)["reasoning_tokens"] = 999
	first, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	u := first.Items[0].Usage
	if *u.PromptTokens != 11 || *u.CompletionTokens != 29 || *u.ReasoningTokens != 9 || *u.TotalTokens != 40 || *u.AccumulatedTokens != 2551 || u.Source != "upstream" {
		t.Fatalf("wrong list summary: %+v", u)
	}
	*u.TotalTokens = 999
	detail, err := s.Get(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	detail.Usage["completion_tokens_details"].(map[string]any)["reasoning_tokens"] = 888
	for _, store := range []*Store{s, New(path)} {
		snapshot, err := store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		u := snapshot.Items[0].Usage
		if *u.TotalTokens != 40 || *u.ReasoningTokens != 9 || *u.AccumulatedTokens != 2551 {
			t.Fatalf("usage changed after mutation/reload: %+v", u)
		}
	}
}
