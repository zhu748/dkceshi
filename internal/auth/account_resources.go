package auth

import "sync"

// Lease expiry can retire resources while a control request is finishing.
// Only exchange callbacks under this lock; queueing/I/O always runs outside it.
var accountResourceMu sync.Mutex

// SetAccountCleanup lets the resource-owning runtime register cleanup without
// making authentication depend on sessions or any interface protocol.
func (a *RequestAuth) SetAccountCleanup(cleanup func()) {
	if cleanup == nil {
		a.SetAccountFinalizer(nil)
		return
	}
	a.SetAccountFinalizer(func(bool) { cleanup() })
}

// Finalizers distinguish a retired session from normal request completion.
// Retiring one session must never run an account-wide deletion policy.
func (a *RequestAuth) SetAccountFinalizer(cleanup func(final bool)) {
	if a != nil {
		accountResourceMu.Lock()
		previous := a.accountCleanup
		a.accountCleanup = cleanup
		accountResourceMu.Unlock()
		if previous != nil {
			previous(false)
		}
	}
}

// Cleanup runs before changing credentials/owner, at most once. Callbacks must
// snapshot any required credentials before queuing asynchronous work.
func (a *RequestAuth) CleanupAccountResources() {
	a.finishAccountResources(false)
}

// FinishAccountResources reports whether this request owned a cleanup callback.
// Adapters can retain legacy cleanup for alternate callers without deleting twice.
func (a *RequestAuth) FinishAccountResources() bool {
	return a.finishAccountResources(true)
}

func (a *RequestAuth) finishAccountResources(final bool) bool {
	if a != nil {
		accountResourceMu.Lock()
		cleanup := a.accountCleanup
		a.accountCleanup = nil
		accountResourceMu.Unlock()
		if cleanup != nil {
			cleanup(final)
			return true
		}
	}
	return false
}
