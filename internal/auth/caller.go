package auth

import "net/http"

// DetermineCaller resolves caller identity without acquiring any pooled account.
// Use this for local-cache lookup routes that only need tenant isolation.
func (r *Resolver) DetermineCaller(req *http.Request) (*RequestAuth, error) {
	callerKey := extractCallerToken(req)
	if callerKey == "" {
		return nil, ErrUnauthorized
	}
	callerID := callerTokenID(callerKey)
	a := &RequestAuth{
		UseConfigToken: false,
		CallerID:       callerID,
		UserAgent:      callerUserAgent(req),
		ToolMarker:     toolMarkerFor(r, callerID),
		resolver:       r,
		TriedAccounts:  map[string]bool{},
	}
	if r == nil || r.Store == nil || !r.Store.HasAPIKey(callerKey) {
		a.DeepSeekToken = callerKey
	} else {
		a.ManagedCredential = true
	}
	return a, nil
}

// If an integration callback panics, the HTTP recoverer may handle it before
// the caller has received an auth to release. Return that slot before unwinding.
func (r *Resolver) completeAccountAcquisition(a *RequestAuth) {
	transferred := false
	defer func() {
		if !transferred {
			r.Release(a)
		}
	}()
	a.markAccountUsed()
	if r.OnAccountUsed != nil {
		r.OnAccountUsed(a.AccountID)
	}
	transferred = true
}
