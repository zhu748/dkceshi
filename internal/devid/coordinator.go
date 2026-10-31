package devid

import (
	"context"
	"sync"
)

// MaxConcurrency applies to all accounts and all refresh/login entry points.
const MaxConcurrency = 2

var generationSlots = make(chan struct{}, MaxConcurrency)
var accountLocks = struct {
	sync.Mutex
	items map[string]*accountLock
}{items: make(map[string]*accountLock)}

type accountLock struct {
	gate chan struct{}
	refs int
}

// AcquireAccount is cancellable and keeps waiters on the same lock even when
// an account is deleted. Entries disappear only after the final holder/waiter.
func AcquireAccount(ctx context.Context, identifier string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	accountLocks.Lock()
	entry := accountLocks.items[identifier]
	if entry == nil {
		entry = &accountLock{gate: make(chan struct{}, 1)}
		accountLocks.items[identifier] = entry
	}
	entry.refs++
	accountLocks.Unlock()
	forget := func() {
		accountLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(accountLocks.items, identifier)
		}
		accountLocks.Unlock()
	}
	select {
	case entry.gate <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-entry.gate; forget() }) }, nil
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
}

func AcquireGeneration(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case generationSlots <- struct{}{}:
		return func() { <-generationSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
