package completionruntime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

type failureCleanupDS struct{ ownedCleanupDS }

func (*failureCleanupDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	return "", fmt.Errorf("network unavailable after session creation")
}

type noFileCleanupPolicy struct{ cleanupPolicy }

func (noFileCleanupPolicy) CurrentInputFileEnabled() bool { return false }

func TestFailedAlternateStartReturnsAndCleansItsOwnSession(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	defer pool.Close()
	resolver := auth.NewResolver(store, pool, func(_ context.Context, acc config.Account) (string, error) { return acc.Token, nil })
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer key")
	a, err := resolver.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Release(a)
	originalOwner := a.AccountID
	ds := &failureCleanupDS{ownedCleanupDS{events: make(chan ownedDeletion, 4)}}
	result, outErr := ExecuteNonStreamStartedWithRetry(context.Background(), ds, a, StartResult{
		SessionID: "old-session", Payload: map[string]any{"chat_session_id": "old-session"}, Pow: "pow",
		Request:  promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "hello"},
		Response: sseHTTPResponse(http.StatusOK, "data: [DONE]"),
	}, Options{RetryEnabled: true, CurrentInputFile: noFileCleanupPolicy{cleanupPolicy{mode: "single"}}})
	if outErr == nil || a.AccountID == originalOwner || result.SessionID != "session-1" {
		t.Fatalf("failed start lost its owner/session: result=%+v owner=%s error=%v", result, a.AccountID, outErr)
	}
	resolver.Release(a)
	select {
	case got := <-ds.events:
		if got.owner != a.AccountID || got.session != result.SessionID || got.err != nil {
			t.Fatalf("wrong cleanup identity: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("failed replacement session leaked")
	}
	a.FinalizeAccountResources()
	select {
	case got := <-ds.events:
		t.Fatalf("cleanup repeated after release: %+v", got)
	default:
	}
}

type allCleanupDS struct {
	ownedCleanupDS
	allOwner string
}

func (d *allCleanupDS) DeleteAllSessions(_ context.Context, owner *auth.RequestAuth) error {
	d.allOwner = owner.AccountID
	return nil
}

func TestSessionFinalizationUsesAllPolicyOnlyAtRequestCompletion(t *testing.T) {
	ds := &allCleanupDS{ownedCleanupDS: ownedCleanupDS{events: make(chan ownedDeletion, 4)}}
	a := &auth.RequestAuth{AccountID: "owner", DeepSeekToken: "token"}
	for range 2 {
		if _, err := createCompletionSession(context.Background(), ds, a, 1, cleanupPolicy{mode: "all"}); err != nil {
			t.Fatal(err)
		}
	}
	if ds.allOwner != "" {
		t.Fatal("replacing one session deleted all sessions")
	}
	a.FinalizeAccountResources()
	if ds.allOwner != "owner" {
		t.Fatal("final delete-all missed its owner")
	}
	select {
	case got := <-ds.events:
		if got.session != "session-1" {
			t.Fatalf("wrong retired session: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("retired session was not cleaned")
	}
}
