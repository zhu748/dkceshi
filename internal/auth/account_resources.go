package auth

import "sync"

// Lease expiry can retire resources while a control request is finishing.
// Only exchange callbacks under this lock; queueing/I/O always runs outside it.
var accountResourceMu sync.Mutex

// SetAccountCleanup lets the resource-owning runtime register cleanup without
// making authentication depend on sessions or any interface protocol.
func (a *RequestAuth) SetAccountCleanup(cleanup func()) {
	if a != nil {
		accountResourceMu.Lock()
		previous := a.accountCleanup
		a.accountCleanup = cleanup
		accountResourceMu.Unlock()
		if previous != nil {
			previous()
		}
	}
}

// Cleanup runs before changing credentials/owner, at most once. Callbacks must
// snapshot any required credentials before queuing asynchronous work.
func (a *RequestAuth) CleanupAccountResources() {
	if a != nil {
		accountResourceMu.Lock()
		cleanup := a.accountCleanup
		a.accountCleanup = nil
		accountResourceMu.Unlock()
		if cleanup != nil {
			cleanup()
		}
	}
}
