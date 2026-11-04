package completionruntime

import (
	"context"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

// Register the session while its account is pinned. A replacement session on
// the same owner or a switch retires it through the existing bounded cleanup
// queue. Final request release applies the captured single/all policy.
func createCompletionSession(ctx context.Context, ds DeepSeekCaller, a *auth.RequestAuth, attempts int, settings any) (string, error) {
	policy, ok := settings.(interface{ AutoDeleteMode() string })
	mode := "none"
	if ok {
		mode = policy.AutoDeleteMode()
	}
	cleanup := mode != "none" && a != nil
	if cleanup {
		a.CleanupAccountResources()
	}
	id, err := ds.CreateSession(ctx, a, attempts)
	if err == nil && id != "" && cleanup && canDeleteCompletionSession(ds) {
		owner := &auth.RequestAuth{AccountID: a.AccountID, DeepSeekToken: a.DeepSeekToken, Account: a.Account, CallerID: a.CallerID, UserAgent: a.UserAgent, ToolMarker: a.ToolMarker}
		refreshOwner := func() {
			// Read fresh credentials for this owner without touching the live
			// request, which may already be refreshing or switching accounts.
			if store, ok := settings.(interface {
				FindAccount(string) (config.Account, bool)
			}); ok {
				if current, found := store.FindAccount(owner.AccountID); found && current.Token != "" {
					owner.Account, owner.DeepSeekToken = current, current.Token
				}
			}
		}
		a.SetAccountCleanup(func() {
			refreshOwner()
			enqueueRepairSessionDelete(ds, owner, id)
		}, func() {
			refreshOwner()
			deleteCompletionSession(ds, owner, id, mode)
		})
	}
	return id, err
}
