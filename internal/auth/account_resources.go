package auth

import "sync"

// Lease expiry can retire resources while a control request is finishing.
// Only exchange callbacks under this lock; queueing/I/O always runs outside it.
var accountResourceMu sync.Mutex

// SetAccountCleanup lets the resource-owning runtime register cleanup without
// making authentication depend on sessions or any interface protocol.
func (a *RequestAuth) SetAccountCleanup(cleanup func(), finalizers ...func()) {
	if a != nil {
		accountResourceMu.Lock()
		previous := a.accountCleanup
		a.resourcesManaged = true
		a.accountCleanup = cleanup
		a.accountFinalize = cleanup
		if len(finalizers) > 0 {
			a.accountFinalize = finalizers[0]
		}
		var late func()
		if a.resourcesFinalized {
			late = a.accountFinalize
			a.accountCleanup, a.accountFinalize = nil, nil
		}
		accountResourceMu.Unlock()
		if previous != nil {
			previous()
		}
		if late != nil {
			late()
		}
	}
}

// Cleanup runs before changing credentials/owner, at most once. Callbacks must
// snapshot any required credentials before queuing asynchronous work.
func (a *RequestAuth) CleanupAccountResources() {
	a.finishAccountResources(false)
}

// FinalizeAccountResources retires the last registered owner even when startup
// failed before a handler received its session ID. Finalization and replacement
// compete for the same callback pair, so only one cleanup policy can run.
// The result reports shared ownership, including a session already retired by a
// failed switch, so legacy handler cleanup does not delete it a second time.
func (a *RequestAuth) FinalizeAccountResources() bool {
	return a.finishAccountResources(true)
}

func (a *RequestAuth) finishAccountResources(final bool) bool {
	if a != nil {
		accountResourceMu.Lock()
		cleanup := a.accountCleanup
		if final {
			a.resourcesFinalized = true
			cleanup = a.accountFinalize
		}
		a.accountCleanup = nil
		a.accountFinalize = nil
		managed := a.resourcesManaged
		accountResourceMu.Unlock()
		if cleanup != nil {
			cleanup()
			return true
		}
		return final && managed
	}
	return false
}
