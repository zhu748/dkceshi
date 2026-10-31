package auth

import (
	"fmt"
	"net/http"
	"testing"
)

func TestRequestErrorStatusPreservesQuotaClassification(t *testing.T) {
	for _, err := range []error{ErrNoAccount, fmt.Errorf("acquire: %w", ErrNoIdleAccount)} {
		if got := RequestErrorStatus(err); got != http.StatusTooManyRequests {
			t.Fatalf("pool exhaustion became %d", got)
		}
	}
	if got := RequestErrorStatus(ErrUnauthorized); got != http.StatusUnauthorized {
		t.Fatalf("authentication became %d", got)
	}
}
