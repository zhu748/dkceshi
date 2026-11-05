package proxybridge

import (
	"errors"
	"strings"
	"testing"
)

func TestHelperDiagnosticsOnlyAcceptControlledCodes(t *testing.T) {
	d := &helperDiagnostics{}
	for _, text := range []string{"password=private\nDS2API_PROXY_", "ERROR server_tls_failed_check_ca_sni\n", strings.Repeat("secret", 1000) + "\nDS2API_PROXY_ERROR echoed private\n"} {
		if _, err := d.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	message := d.failure("fallback")
	if !strings.Contains(message, "TLS") || strings.Contains(message, "private") {
		t.Fatal(message)
	}
}

func TestFailedStartBackoffIsBoundedAndIsolated(t *testing.T) {
	CloseAll()
	defer CloseAll()
	rememberStartFailure("old-credentials", errors.New("fixture"))
	if cachedStartFailure("old-credentials") == nil || cachedStartFailure("new-credentials") != nil {
		t.Fatal("backoff isolation")
	}
	for i := 0; i < 200; i++ {
		rememberStartFailure(strings.Repeat("x", i), errors.New("fixture"))
	}
	registry.Lock()
	n := len(registry.failures)
	registry.Unlock()
	if n > 128 {
		t.Fatal("unbounded failure cache")
	}
}
