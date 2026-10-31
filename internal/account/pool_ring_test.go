package account

import (
	"testing"

	"ds2api/internal/config"
)

// The rotation ring must preserve the exact round-robin semantics of the old
// "remove from position, append to tail" queue, both for head picks (the
// common tryAcquire path) and for middle picks (targeted acquire).
func TestBumpQueueRingRotationSemantics(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	p := &Pool{queue: append([]string(nil), ids...), queueHead: 0}

	// Reference implementation of the old behavior.
	oldBump := func(q []string, id string) []string {
		out := append([]string(nil), q...)
		for i, v := range out {
			if v != id {
				continue
			}
			out = append(out[:i], out[i+1:]...)
			out = append(out, id)
			return out
		}
		return out
	}

	// Head picks and middle picks in a deterministic sequence.
	picks := []string{"a", "c", "a", "e", "b", "d", "a"}
	ref := append([]string(nil), ids...)
	for i, pick := range picks {
		p.bumpQueue(pick)
		ref = oldBump(ref, pick)
		got := p.queueRingIDs()
		if len(got) != len(ref) {
			t.Fatalf("pick %d (%s): length mismatch got=%v want=%v", i, pick, got, ref)
		}
		for j := range ref {
			if got[j] != ref[j] {
				t.Fatalf("pick %d (%s): order mismatch got=%v want=%v", i, pick, got, ref)
			}
		}
	}

	// Full wrap: repeatedly bumping the head must cycle through every account
	// before repeating (fairness).
	p = &Pool{queue: append([]string(nil), ids...), queueHead: 0}
	seen := map[string]int{}
	for i := 0; i < len(ids)*3; i++ {
		head := p.queueRingIDs()[0]
		seen[head]++
		p.bumpQueue(head)
	}
	for _, id := range ids {
		if seen[id] != 3 {
			t.Fatalf("expected each account to be head exactly 3 times, got %v", seen)
		}
	}
}

// The single-account queue must keep working (modulo edge case).
func TestBumpQueueRingSingleAccount(t *testing.T) {
	p := &Pool{queue: []string{"only"}, queueHead: 0}
	for i := 0; i < 5; i++ {
		p.bumpQueue("only")
		if got := p.queueRingIDs(); len(got) != 1 || got[0] != "only" {
			t.Fatalf("single-account ring broken: %v", got)
		}
	}
}

// Benchmark: acquire on a large pool pays O(1) for the rotation instead of the
// previous full-slice memmove.
func BenchmarkAcquireRotationLargePool(b *testing.B) {
	const n = 5000
	b.Setenv("DS2API_ACCOUNT_MAX_INFLIGHT", "2")
	b.Setenv("DS2API_ACCOUNT_MAX_QUEUE", "")
	accountsJSON := make([]byte, 0, n*48)
	accountsJSON = append(accountsJSON, `{"keys":["k1"],"accounts":[`...)
	for i := 0; i < n; i++ {
		if i > 0 {
			accountsJSON = append(accountsJSON, ',')
		}
		accountsJSON = append(accountsJSON, `{"email":"acc-`...)
		accountsJSON = appendAccountsDigits(accountsJSON, i)
		accountsJSON = append(accountsJSON, `@example.com","token":"t"}`...)
	}
	accountsJSON = append(accountsJSON, ']')
	accountsJSON = append(accountsJSON, '}')
	b.Setenv("DS2API_CONFIG_JSON", string(accountsJSON))
	store := config.LoadStore()
	p := NewPool(store)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		acc, ok := p.Acquire("", nil, nil)
		if !ok {
			b.Fatalf("acquire failed at iteration %d", i)
		}
		p.Release(acc.Identifier())
	}
}

// bumpQueueLegacy is the pre-ring implementation, kept here so the benchmark
// below can quantify the difference under identical conditions.
func (p *Pool) bumpQueueLegacy(accountID string) {
	for i, id := range p.queue {
		if id != accountID {
			continue
		}
		p.queue = append(p.queue[:i], p.queue[i+1:]...)
		p.queue = append(p.queue, accountID)
		return
	}
}

// BenchmarkBumpQueueLegacyVsRing isolates just the rotation cost.
func BenchmarkBumpQueueLegacyVsRing(b *testing.B) {
	ids := make([]string, 0, 5000)
	for i := 0; i < 5000; i++ {
		ids = append(ids, "acc-"+string(rune('0'+i%10))+string(rune('0'+i/10%10))+string(rune('0'+i/100%10))+string(rune('0'+i/1000%10)))
	}
	b.Run("legacy", func(b *testing.B) {
		p := &Pool{queue: append([]string(nil), ids...), queueHead: 0}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Rotate the head like tryAcquire does.
			p.bumpQueueLegacy(p.queue[0])
		}
	})
	b.Run("ring", func(b *testing.B) {
		p := &Pool{queue: append([]string(nil), ids...), queueHead: 0}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p.bumpQueue(p.queue[p.queueHead])
		}
	})
}

func appendAccountsDigits(buf []byte, v int) []byte {
	if v == 0 {
		return append(buf, '0')
	}
	var tmp [10]byte
	pos := len(tmp)
	for v > 0 {
		pos--
		tmp[pos] = byte('0' + v%10)
		v /= 10
	}
	return append(buf, tmp[pos:]...)
}
