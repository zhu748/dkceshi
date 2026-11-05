package mihomo

import (
	"ds2api/internal/account"
	"ds2api/internal/config"
	"testing"
	"time"
)

type editingPool struct {
	fakePool
	edit func()
}

func (p *editingPool) PauseForProxyUpdate() (<-chan struct{}, func()) {
	if p.edit != nil {
		fn := p.edit
		p.edit = nil
		fn()
	}
	ready := make(chan struct{})
	close(ready)
	return ready, func() {}
}

func TestReconcileDoesNotOverwriteConcurrentManualChoices(t *testing.T) {
	for _, mode := range []string{"manual", "direct", "changed", "auto_off"} {
		t.Run(mode, func(t *testing.T) {
			store := newAutoBindTestStore(t)
			pool := &editingPool{}
			mgr := NewManager(store, pool)
			defer mgr.Stop()
			hk, jp, _ := nodeKeys3()
			autoBind(t, mgr, store, "a@test.com", hk)
			enableAutoBridge(t, store)
			markHealth(mgr, hk, NodeHealthFail, 0, 2)
			markHealth(mgr, jp, NodeHealthOK, 10, 0)
			pool.edit = func() {
				if err := store.Update(func(c *config.Config) error {
					if mode == "auto_off" {
						c.Mihomo.AutoBind = false
						return nil
					}
					i := findAccountIndexLocked(c, "a@test.com")
					c.Accounts[i].ProxyID = "manual-1"
					switch mode {
					case "manual":
						c.Accounts[i].ManualBinding = true
					case "direct":
						c.Accounts[i].ProxyID = ""
						c.Accounts[i].NoProxy = true
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			mgr.reconcileBindings()
			a, _ := store.FindAccount("a@test.com")
			want := "manual-1"
			if mode == "auto_off" {
				want = config.MihomoManagedProxyID(hk)
			}
			if mode == "direct" {
				want = ""
			}
			if a.ProxyID != want {
				t.Fatalf("%s choice overwritten: %+v", mode, a)
			}
		})
	}
}

func TestRealPoolDrainsThenReconcilesWithoutWaitingForNextTick(t *testing.T) {
	store := newAutoBindTestStore(t)
	pool := account.NewPool(store)
	defer pool.Close()
	mgr := NewManager(store, pool)
	defer mgr.Stop()
	enableAutoBridge(t, store)
	a, ok := pool.Acquire("a@test.com", nil, nil)
	if !ok {
		t.Fatal("acquire")
	}
	if mgr.reconcileBindings() {
		t.Fatal("updated with an active request")
	}
	if _, ok := pool.Acquire("b@test.com", nil, nil); ok {
		t.Fatal("new requests bypassed drain")
	}
	pool.Release(a.Identifier())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mgr.drainMu.Lock()
		pending := mgr.drainPending
		mgr.drainMu.Unlock()
		acc, _ := store.FindAccount("a@test.com")
		if !pending && acc.ProxyID != "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("drained pool was not reconciled immediately")
}
