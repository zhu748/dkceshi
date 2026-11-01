package auth

import (
	"context"
	"errors"
)

type loginFlight struct {
	done chan struct{}
	auth RequestAuth
	err  error
}

func (r *Resolver) loginAndPersist(ctx context.Context, a *RequestAuth) error {
	return r.loginCoalesced(ctx, a, false)
}

// Coalesce by account, reusing tokens that another request already refreshed.
// A waiting caller can cancel independently; if the leader is cancelled, live
// waiters elect a fresh leader using their own context.
func (r *Resolver) loginCoalesced(ctx context.Context, a *RequestAuth, force bool) error {
	if r == nil || r.Login == nil || r.Store == nil || a == nil {
		return errors.New("account login is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.mu.Lock()
		if flight := r.loginFlights[a.AccountID]; flight != nil {
			r.mu.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-flight.done:
			}
			if errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) {
				continue
			}
			if flight.err == nil {
				copyLoginCredentials(a, &flight.auth)
			}
			return flight.err
		}
		if stored, ok := r.Store.FindAccount(a.AccountID); ok && stored.Token != "" && stored.Token != a.Account.Token {
			a.Account.Token, a.DeepSeekToken = stored.Token, stored.Token
			a.Account.DeviceID, a.Account.DeviceIDType = stored.DeviceID, stored.DeviceIDType
			r.mu.Unlock()
			return nil
		}
		if r.loginFlights == nil {
			r.loginFlights = make(map[string]*loginFlight)
		}
		flight := &loginFlight{done: make(chan struct{})}
		r.loginFlights[a.AccountID] = flight
		r.mu.Unlock()

		return r.performLoginFlight(ctx, a, force, flight)
	}
}

// Publish even when the login implementation panics or exits its goroutine:
// request-level recovery must not strand every future waiter for this account.
func (r *Resolver) performLoginFlight(ctx context.Context, a *RequestAuth, force bool, flight *loginFlight) (err error) {
	completed := false
	defer func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !completed {
			err = errors.New("account login interrupted")
		}
		flight.auth, flight.err = *a, err
		delete(r.loginFlights, a.AccountID)
		close(flight.done)
	}()
	if force {
		err = r.Store.UpdateAccountToken(a.AccountID, "")
		if err == nil {
			a.Account.Token = ""
		}
	}
	if err == nil {
		err = r.loginAndPersistOnce(ctx, a)
	}
	completed = true
	return err
}

func copyLoginCredentials(dst, src *RequestAuth) {
	dst.DeepSeekToken = src.DeepSeekToken
	dst.Account.Token = src.Account.Token
	dst.Account.DeviceID = src.Account.DeviceID
	dst.Account.DeviceIDType = src.Account.DeviceIDType
}

func (r *Resolver) loginAndPersistOnce(ctx context.Context, a *RequestAuth) error {
	if r == nil || r.Login == nil {
		return errors.New("account login is unavailable")
	}
	token, err := r.Login(ctx, a.Account)
	if err != nil {
		return err
	}
	a.Account.Token = token
	if stored, ok := r.Store.FindAccount(a.AccountID); ok {
		a.Account.DeviceID = stored.DeviceID
		a.Account.DeviceIDType = stored.DeviceIDType
	}
	a.DeepSeekToken = token
	r.markTokenRefreshedNow(a.AccountID)
	if err := r.Store.UpdateAccountToken(a.AccountID, token); err != nil {
		return err
	}
	if r.PostLogin != nil {
		r.PostLogin(ctx, a)
	}
	return nil
}
