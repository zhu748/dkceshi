package completionruntime

import (
	"context"
	"sync"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type repairCleanupJob struct {
	ds      DeepSeekCaller
	auth    *auth.RequestAuth
	session string
	all     bool
}

var repairCleanup = struct {
	sync.Once
	jobs chan repairCleanupJob
}{}

// Cleanup never delays tool delivery. A fixed queue and two workers bound
// goroutine count even when the remote delete endpoint is unavailable.
func enqueueRepairSessionDelete(ds DeepSeekCaller, a *auth.RequestAuth, session string) {
	enqueueSessionCleanup(ds, a, session, false)
}

type tokenSessionDeleter interface {
	DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error)
}
type allSessionDeleter interface {
	DeleteAllSessionsForToken(context.Context, string) error
}

func enqueueSessionCleanup(ds DeepSeekCaller, a *auth.RequestAuth, session string, all bool) {
	_, byAuth := ds.(repairSessionDeleter)
	_, byToken := ds.(tokenSessionDeleter)
	_, deleteAll := ds.(allSessionDeleter)
	canDelete := byAuth || byToken || all && deleteAll
	if a == nil || session == "" || !canDelete {
		return
	}
	repairCleanup.Do(func() {
		repairCleanup.jobs = make(chan repairCleanupJob, 128)
		for i := 0; i < 2; i++ {
			go func() {
				for job := range repairCleanup.jobs {
					deleteOwnedSession(job)
				}
			}()
		}
	})
	// Snapshot only credentials needed for deletion; the live request auth may
	// be released or switched to another account as soon as delivery completes.
	snapshot := &auth.RequestAuth{DeepSeekToken: a.DeepSeekToken, AccountID: a.AccountID, Account: a.Account, CallerID: a.CallerID, UserAgent: a.UserAgent, ToolMarker: a.ToolMarker}
	select {
	case repairCleanup.jobs <- repairCleanupJob{ds: ds, auth: snapshot, session: session, all: all}:
	default:
		config.Logger.Warn("[toolcall_repair] cleanup queue full", "session_id", session)
	}
}

func deleteOwnedSession(job repairCleanupJob) {
	ctx, cancel := context.WithTimeout(auth.WithAuth(context.Background(), job.auth), toolCallRepairSessionDeleteTimeout)
	defer cancel()
	var err error
	if all, ok := job.ds.(allSessionDeleter); job.all && ok {
		err = all.DeleteAllSessionsForToken(ctx, job.auth.DeepSeekToken)
	} else if deleter, ok := job.ds.(repairSessionDeleter); ok {
		_, err = deleter.DeleteSession(ctx, job.auth, job.session, 1)
	} else if deleter, ok := job.ds.(tokenSessionDeleter); ok {
		_, err = deleter.DeleteSessionForToken(ctx, job.auth.DeepSeekToken, job.session)
	}
	if err != nil {
		config.Logger.Warn("[session_cleanup] deletion failed", "session_id", job.session, "all", job.all, "error", err)
	}
}
