package completionruntime

import (
	"context"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type tokenSessionDeleter interface {
	DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error)
}

func canDeleteCompletionSession(ds DeepSeekCaller) bool {
	_, native := ds.(repairSessionDeleter)
	_, token := ds.(tokenSessionDeleter)
	return native || token
}

// Final deletion shares the originating account's route and fresh credentials.
// Replacement sessions use the single-session queue, including in "all" mode;
// only request completion applies the configured delete-all policy.
func deleteCompletionSession(ds DeepSeekCaller, owner *auth.RequestAuth, id, mode string) {
	if mode != "all" {
		deleteRepairSession(context.Background(), ds, owner, id)
		return
	}
	ctx, cancel := context.WithTimeout(auth.WithAuth(context.Background(), owner), toolCallRepairSessionDeleteTimeout)
	defer cancel()
	var err error
	switch deleter := ds.(type) {
	case interface {
		DeleteAllSessions(context.Context, *auth.RequestAuth) error
	}:
		err = deleter.DeleteAllSessions(ctx, owner)
	case interface {
		DeleteAllSessionsForToken(context.Context, string) error
	}:
		err = deleter.DeleteAllSessionsForToken(ctx, owner.DeepSeekToken)
	default:
		deleteRepairSession(ctx, ds, owner, id)
	}
	if err != nil {
		config.Logger.Warn("[completion_runtime] final session cleanup failed", "account", owner.AccountID, "mode", mode, "error", err)
	}
}
