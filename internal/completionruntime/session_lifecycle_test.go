package completionruntime

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"fmt"
	"testing"
	"time"
)

type cleanupPolicy struct {
	currentInputRuntimeConfig
	mode string
}

func (p cleanupPolicy) AutoDeleteMode() string { return p.mode }

type ownedDeletion struct {
	owner, token, session string
	err                   error
}
type ownedCleanupDS struct {
	fakeDeepSeekCaller
	created int
	events  chan ownedDeletion
}

func (d *ownedCleanupDS) CreateSession(context.Context, *auth.RequestAuth, int) (string, error) {
	d.created++
	return fmt.Sprintf("session-%d", d.created), nil
}
func (d *ownedCleanupDS) DeleteSession(ctx context.Context, a *auth.RequestAuth, id string, _ int) (*dsclient.DeleteSessionResult, error) {
	d.events <- ownedDeletion{a.AccountID, a.DeepSeekToken, id, ctx.Err()}
	return &dsclient.DeleteSessionResult{Success: true}, nil
}
func TestRetiredSessionsUseTheirOriginalOwnerAndDetachedContext(t *testing.T) {
	for _, mode := range []string{"single", "all"} {
		t.Run(mode, func(t *testing.T) {
			d := &ownedCleanupDS{events: make(chan ownedDeletion, 4)}
			a := &auth.RequestAuth{AccountID: "a", DeepSeekToken: "token-a"}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			for range 2 {
				if _, err := createCompletionSession(ctx, d, a, 1, cleanupPolicy{mode: mode}); err != nil {
					t.Fatal(err)
				}
			}
			a.CleanupAccountResources()
			a.CleanupAccountResources()
			a.AccountID, a.DeepSeekToken = "b", "token-b"
			seen := map[string]bool{}
			for i := 1; i <= 2; i++ {
				select {
				case got := <-d.events:
					if got.owner != "a" || got.token != "token-a" || (got.session != "session-1" && got.session != "session-2") || seen[got.session] || got.err != nil {
						t.Fatalf("wrong cleanup: %+v", got)
					}
					seen[got.session] = true
				case <-time.After(time.Second):
					t.Fatal("session cleanup did not run")
				}
			}
		})
	}
}
func TestSessionCleanupRespectsDisabledPolicy(t *testing.T) {
	d := &ownedCleanupDS{events: make(chan ownedDeletion, 2)}
	a := &auth.RequestAuth{AccountID: "a", DeepSeekToken: "token"}
	if _, err := createCompletionSession(context.Background(), d, a, 1, cleanupPolicy{mode: "none"}); err != nil {
		t.Fatal(err)
	}
	a.CleanupAccountResources()
	select {
	case got := <-d.events:
		t.Fatalf("unexpected deletion: %+v", got)
	default:
	}
}

type freshCleanupPolicy struct {
	cleanupPolicy
	store *config.Store
}

func (p freshCleanupPolicy) FindAccount(id string) (config.Account, bool) {
	return p.store.FindAccount(id)
}
func TestSessionCleanupUsesRefreshedTokenForItsOriginalOwner(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"a@example.com","token":"old"}]}`)
	store := config.LoadStore()
	d := &ownedCleanupDS{events: make(chan ownedDeletion, 1)}
	a := &auth.RequestAuth{AccountID: "a@example.com", DeepSeekToken: "old"}
	if _, err := createCompletionSession(context.Background(), d, a, 1, freshCleanupPolicy{cleanupPolicy{mode: "single"}, store}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAccountToken("a@example.com", "fresh"); err != nil {
		t.Fatal(err)
	}
	a.CleanupAccountResources()
	a.DeepSeekToken = "other-owner-token"
	a.AccountID = "other@example.com"
	select {
	case got := <-d.events:
		if got.owner != "a@example.com" || got.token != "fresh" {
			t.Fatalf("wrong owner credentials: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not run")
	}
}
