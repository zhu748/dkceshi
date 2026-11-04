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

func TestFinalizationAndReplacementChooseOneResourceCallback(t *testing.T) {
	var retired, finished atomic.Int64
	for range 100 {
		a := &RequestAuth{}
		a.SetAccountCleanup(func() { retired.Add(1) }, func() { finished.Add(1) })
		var workers sync.WaitGroup
		workers.Go(a.CleanupAccountResources)
		workers.Go(func() { a.FinalizeAccountResources() })
		workers.Wait()
		a.FinalizeAccountResources()
	}
	if retired.Load()+finished.Load() != 100 {
		t.Fatalf("callbacks repeated or lost: retired=%d final=%d", retired.Load(), finished.Load())
	}
}

func TestRegistrationAfterReleaseImmediatelyFinalizesLateResources(t *testing.T) {
	a := &RequestAuth{}
	a.FinalizeAccountResources()
	calls := 0
	a.SetAccountCleanup(func() { t.Error("late resource was treated as a replacement") }, func() { calls++ })
	if !a.FinalizeAccountResources() || calls != 1 {
		t.Fatal("late resource leaked or was deleted twice")
	}
}

func TestReleaseFinalizesDirectTokenResources(t *testing.T) {
	a := &RequestAuth{}
	calls := 0
	a.SetAccountCleanup(func() { t.Error("replacement cleanup used at request completion") }, func() { calls++ })
	resolver := &Resolver{}
	resolver.Release(a)
	resolver.Release(a)
	if calls != 1 {
		t.Fatalf("final cleanup count=%d", calls)
	}
}
