package account

import (
	"testing"
	"time"
)

func TestProxyDrainBlocksNewLeasesUntilUpdateFinishes(t *testing.T) {
	p := newPoolForTest(t, "2")
	defer p.Close()
	a, ok := p.Acquire("", nil, nil)
	if !ok {
		t.Fatal("acquire")
	}
	ready, finish := p.PauseForProxyUpdate()
	defer finish()
	if _, ok := p.Acquire("", nil, nil); ok {
		t.Fatal("new request raced with the update")
	}
	if _, ok := p.AcquireKeepAlive("acc2@example.com"); ok {
		t.Fatal("keepalive bypassed drain")
	}
	select {
	case <-ready:
		t.Fatal("active request was not drained")
	default:
	}
	done := make(chan bool, 1)
	go func() {
		b, ok := p.AcquireWait(t.Context(), "", nil, nil)
		if ok {
			p.Release(b.Identifier())
		}
		done <- ok
	}()
	waitForWaitingCount(t, p, 1)
	p.Release(a.Identifier())
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("drain not signalled")
	}
	select {
	case <-done:
		t.Fatal("waiter resumed before update finished")
	default:
	}
	finish()
	finish()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("queued request lost")
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not resumed")
	}
}

func TestClosingPoolUnblocksProxyDrain(t *testing.T) {
	p := newPoolForTest(t, "2")
	a, ok := p.Acquire("", nil, nil)
	if !ok {
		t.Fatal("acquire")
	}
	ready, finish := p.PauseForProxyUpdate()
	defer finish()
	p.Close()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("drain leaked on shutdown")
	}
	p.Release(a.Identifier())
}
