package completionruntime

import (
	"context"
	"sync"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

type repairCleanupJob struct {
	ds      DeepSeekCaller
	auth    *auth.RequestAuth
	session string
}

var repairCleanup = struct {
	sync.Once
	jobs chan repairCleanupJob
}{}

// Cleanup never delays tool delivery. A fixed queue and two workers bound
// goroutine count even when the remote delete endpoint is unavailable.
func enqueueRepairSessionDelete(ds DeepSeekCaller, a *auth.RequestAuth, session string) {
	if _, ok := ds.(repairSessionDeleter); !ok || a == nil || session == "" {
		return
	}
	repairCleanup.Do(func() {
		repairCleanup.jobs = make(chan repairCleanupJob, 128)
		for i := 0; i < 2; i++ {
			go func() {
				for job := range repairCleanup.jobs {
					deleteRepairSession(context.Background(), job.ds, job.auth, job.session)
				}
			}()
		}
	})
	// Snapshot only credentials needed for deletion; the live request auth may
	// be released or switched to another account as soon as delivery completes.
	snapshot := &auth.RequestAuth{DeepSeekToken: a.DeepSeekToken, AccountID: a.AccountID, Account: a.Account, CallerID: a.CallerID, UserAgent: a.UserAgent, ToolMarker: a.ToolMarker}
	select {
	case repairCleanup.jobs <- repairCleanupJob{ds: ds, auth: snapshot, session: session}:
	default:
		config.Logger.Warn("[toolcall_repair] cleanup queue full", "session_id", session)
	}
}
