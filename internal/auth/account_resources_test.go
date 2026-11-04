package auth

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentAccountResourceRetirementRunsEveryCallbackOnce(t *testing.T) {
	a := &RequestAuth{}
	var calls atomic.Int64
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			for range 20 {
				a.SetAccountCleanup(func() { calls.Add(1) })
				a.CleanupAccountResources()
			}
		})
	}
	workers.Wait()
	a.CleanupAccountResources()
	if got := calls.Load(); got != 320 {
		t.Fatalf("cleanup calls=%d want=320", got)
	}
}
