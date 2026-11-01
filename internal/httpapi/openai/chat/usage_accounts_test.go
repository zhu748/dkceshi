package chat

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
)

func TestChatUsageUsesFinalAccountWithoutHistory(t *testing.T) {
	for _, stopped := range []bool{false, true} {
		store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
		defer store.Close()
		a := &auth.RequestAuth{UseConfigToken: true, AccountID: "first@example.com", CallerID: "caller:a"}
		s := startChatHistoryWithUsage(nil, store, httptest.NewRequest("POST", "/", nil), a,
			promptcompat.StandardRequest{ResolvedModel: "upstream", ResponseModel: "alias", FinalPrompt: "hello"})
		a.AccountID = "final@example.com"
		if stopped {
			s.stopped("thinking", "answer", "stopped")
			s.stopped("thinking", "answer", "stopped")
		} else {
			s.success(200, "", "answer", "stop", map[string]any{"prompt_tokens": 10})
			s.success(200, "", "answer", "stop", map[string]any{"prompt_tokens": 10})
		}
		entries := store.Summary()
		if len(entries) != 1 || entries[0].Calls != 1 || entries[0].Model != "upstream" || entries[0].AccountLabel != "f***@example.com" {
			t.Fatalf("bad Chat accounting: %+v", entries)
		}
	}
}

func TestVercelReleaseAccountsForFinalLeaseAccount(t *testing.T) {
	store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	h := &Handler{Usage: store}
	a := &auth.RequestAuth{UseConfigToken: true, AccountID: "first@example.com", CallerID: "caller:a"}
	id := h.holdStreamLease(a, promptcompat.StandardRequest{ResolvedModel: "upstream", ResponseModel: "alias"}, "session")
	a.AccountID = "final@example.com"
	lease, ok := h.releaseStreamLease(id)
	if !ok {
		t.Fatal("missing stream lease")
	}
	h.recordVercelStreamCompletion(httptest.NewRequest("POST", "/?__stream_release=1", nil), lease.Auth, lease.Standard,
		map[string]any{"finish_reason": "stop", "content": "answer", "usage": map[string]any{"prompt_tokens": 10}})
	if _, ok := h.releaseStreamLease(id); ok {
		t.Fatal("lease can only be released once")
	}
	entries := store.Summary()
	if len(entries) != 1 || entries[0].Calls != 1 || entries[0].AccountLabel != "f***@example.com" {
		t.Fatalf("bad Vercel accounting: %+v", entries)
	}
}

func TestVercelStoppedUsagePreservesNodeRetryAccounting(t *testing.T) {
	store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	h := &Handler{Usage: store}
	usage := map[string]any{"prompt_tokens": 1234, "completion_tokens": 10, "total_tokens": 1244}
	h.recordVercelStreamCompletion(httptest.NewRequest("POST", "/?__stream_release=1", nil), &auth.RequestAuth{CallerID: "caller"}, promptcompat.StandardRequest{FinalPrompt: "short initial prompt"}, map[string]any{"finish_reason": "stopped", "content": "answer", "usage": usage})
	if entries := store.Summary(); len(entries) != 1 || entries[0].PromptTokens != 1234 || entries[0].TotalTokens != 1244 {
		t.Fatalf("Node retry usage was replaced with initial-prompt estimate: %+v", entries)
	}
}
