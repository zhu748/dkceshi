package responsehistory

import (
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSplitHistoryRecordsOriginalUserInputInsteadOfContinuationInstructions(t *testing.T) {
	history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	session := Start(StartParams{Store: history, Request: httptest.NewRequest("POST", "/v1/chat/completions", nil), Auth: &auth.RequestAuth{CallerID: "caller"}, Standard: promptcompat.StandardRequest{
		ArchiveUserInput: "  ORIGINAL USER TASK  ", Messages: []any{map[string]any{"role": "system", "content": "GENERATED CONTINUATION"}}, HistoryText: "history data",
	}})
	if session == nil {
		t.Fatal("archive session missing")
	}
	entry, err := history.Get(session.entryID)
	if err != nil || entry.UserInput != "ORIGINAL USER TASK" {
		t.Fatalf("archive labeled with generated instructions: %q %v", entry.UserInput, err)
	}
}
