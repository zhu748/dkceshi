package auth

import (
	"net/http/httptest"
	"testing"
)

func TestAccountCallbackPanicReturnsItsSlot(t *testing.T) {
	r := newTestResolver(t)
	r.OnAccountUsed = func(string) { panic("integration failure") }
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer managed-key")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected callback panic")
			}
		}()
		_, _ = r.Determine(req)
	}()
	if n := r.Pool.Status()["in_use"].(int); n != 0 {
		t.Fatalf("leaked %d account slots", n)
	}
}
