package chat

import (
	"context"
	"net/http/httptest"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestReleasedFailureObservesOnlyItsCompletionOwner(t *testing.T) {
	for _, ownerMatches := range []bool{true, false} {
		t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"token"}]}`)
		store := config.LoadStore()
		pool := account.NewPool(store)
		t.Cleanup(pool.Close)
		resolver := auth.NewResolver(store, pool, func(context.Context, config.Account) (string, error) { return "token", nil })
		req := httptest.NewRequest("POST", "/", nil)
		req.Header.Set("Authorization", "Bearer key")
		a, err := resolver.Determine(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resolver.Release(a) })
		owner := a.AccountID
		if !ownerMatches {
			owner = "previous-owner"
		}
		h := &Handler{Auth: resolver, Store: store}
		h.observeReleasedAccountFailure(a, map[string]any{"failure_account_id": owner, "error_status": 503, "error_code": "upstream_unavailable", "error": "Upstream service is unavailable and returned no output."})
		acc, _ := store.FindAccount(a.AccountID)
		if acc.IsCoolingDown() != ownerMatches {
			t.Fatal("terminal cooldown was dropped or attributed to another owner")
		}
	}
}
