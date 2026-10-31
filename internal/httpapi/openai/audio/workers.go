package audio

import (
	"net/http"
	"strings"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func (h *Handler) acquireSpeechWorkers(r *http.Request, plan speechPlan, callerAuth *auth.RequestAuth) ([]*auth.RequestAuth, *auth.RequestAuth, *assistantturn.OutputError) {
	runCtx := r.Context()
	seedReq := r
	var err error
	multiAccount := plan.multiAccount
	// Single-account mode needs one concrete account up front so every worker
	// pins to it. Keep that slot instead of releasing and re-acquiring it: the
	// gap let another request take the account, after which the workers failed
	// with "no account" and the whole request returned 502. The seed is handed
	// to the first worker and released once the worker pool is done.
	var seed *auth.RequestAuth
	fixedAccountID := ""
	if !multiAccount {
		seed, err = h.Auth.Determine(seedReq)
		if err != nil {
			status := http.StatusUnauthorized
			if auth.IsNoIdleAccountError(err) {
				status = http.StatusTooManyRequests
			}
			return nil, nil, &assistantturn.OutputError{Status: status, Message: err.Error()}
		}
		fixedAccountID = seed.AccountID
		// Pin the seed too: the recitation runs before SynthesizeSpeech, and an
		// unpinned auth could switch accounts mid-request, which would spread a
		// single-account request over several accounts.
		seed.TargetAccount = fixedAccountID
	}

	workerLimit := h.speechWorkerLimit(plan, callerAuth)
	// Each worker owns one account for the whole request and pulls segments off
	// the queue, so the pool sees one acquire per worker instead of one per
	// segment. Per-minute and 24h account rate limits count every acquire, and
	// acquiring per segment made a single long request burn its own budget and
	// then fail mid-flight once the window was exhausted. RequestAuth's mutable
	// bookkeeping is unsynchronized, which stays safe because a worker's auth
	// is only ever touched by its own goroutine.
	var workersAuth []*auth.RequestAuth
	if seed != nil {
		workersAuth = append(workersAuth, seed)
	}

	// Ensure at least one worker has a valid account (blocking wait if necessary).
	if len(workersAuth) == 0 {
		workerReq := r.Clone(runCtx)
		if fixedAccountID != "" {
			workerReq.Header.Set("X-Ds2-Target-Account", fixedAccountID)
		}
		a, err := h.Auth.Determine(workerReq)
		if err != nil {
			status := http.StatusBadGateway
			if auth.IsNoIdleAccountError(err) {
				status = http.StatusTooManyRequests
			}
			return nil, nil, &assistantturn.OutputError{Status: status, Message: err.Error()}
		}
		workersAuth = append(workersAuth, a)
	}

	// Try to acquire more accounts non-blockingly up to workerLimit for concurrency.
	// If the account pool is at capacity, stop acquiring and process the segments in batches
	// across the ready workers instead of deadlocking or failing.
	for len(workersAuth) < workerLimit {
		workerReq := r.Clone(runCtx)
		if fixedAccountID != "" {
			workerReq.Header.Set("X-Ds2-Target-Account", fixedAccountID)
		}
		var a *auth.RequestAuth
		var err error
		if tryResolver, ok := h.Auth.(interface {
			DetermineTry(*http.Request) (*auth.RequestAuth, error)
		}); ok {
			a, err = tryResolver.DetermineTry(workerReq)
		} else {
			// The required worker already holds a slot. Without a nonblocking
			// acquisition, another Determine can wait on that slot indefinitely.
			break
		}
		if err != nil {
			break
		}
		workersAuth = append(workersAuth, a)
	}

	return workersAuth, seed, nil
}

func (h *Handler) speechWorkerLimit(plan speechPlan, callerAuth *auth.RequestAuth) int {
	segments, multiAccount, perAccount := plan.segments, plan.multiAccount, plan.perAccount
	workerLimit := len(segments)
	if multiAccount {
		accounts := 1
		filter := account.AccountFilter(func(acc config.Account) bool {
			return acc.MatchesPoolType(false)
		})
		if limiter, ok := h.Auth.(interface {
			SchedulableAccountsMatching(account.AccountFilter) int
		}); ok {
			if n := limiter.SchedulableAccountsMatching(filter); n > 0 {
				accounts = n
			}
		} else if limiter, ok := h.Auth.(interface{ SchedulableAccounts() int }); ok {
			if n := limiter.SchedulableAccounts(); n > 0 {
				accounts = n
			}
		}
		if n := accounts * perAccount; n > 0 && n < workerLimit {
			workerLimit = n
		}
	} else if perAccount < workerLimit {
		workerLimit = perAccount
	}
	if limiter, ok := h.Auth.(interface{ GlobalMaxInflight() int }); ok {
		if g := limiter.GlobalMaxInflight(); g > 0 && g < workerLimit {
			workerLimit = g
		}
	}
	// Direct-token callers bypass the pool, so only the per-account limit
	// applies to them.
	if callerAuth != nil && strings.TrimSpace(callerAuth.DeepSeekToken) != "" {
		if workerLimit > perAccount {
			workerLimit = perAccount
		}
	}
	if workerLimit < 1 {
		workerLimit = 1
	}

	return workerLimit
}
