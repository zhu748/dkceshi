package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func pooledTestClient(t *testing.T, handler http.HandlerFunc) (*H2FramedClient, string, *atomic.Int32) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	dials := &atomic.Int32{}
	client := &H2FramedClient{TLSConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	t.Cleanup(client.CloseIdleConnections)
	return client, server.URL, dials
}

func pooledGet(t *testing.T, client *H2FramedClient, url, installation, token string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-device-id", installation)
	req.Header.Set("authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOrderedH2ReusesConnectionsWithoutSharingAccountIdentity(t *testing.T) {
	client, url, dials := pooledTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	})
	for range 6 {
		pooledGet(t, client, url, "installation-a", "token-a")
	}
	if n := dials.Load(); n != 1 {
		t.Fatalf("six sequential requests used %d connections", n)
	}
	pooledGet(t, client, url, "installation-b", "token-a")
	pooledGet(t, client, url, "installation-a", "token-b")
	if n := dials.Load(); n != 3 {
		t.Fatalf("device/token isolation lost: %d connections", n)
	}
	client.CloseIdleConnections()
	pooledGet(t, client, url, "installation-a", "token-a")
	if n := dials.Load(); n != 4 {
		t.Fatalf("closed pool reused an old connection: %d", n)
	}
}

func TestOrderedH2CancellationDoesNotCloseSiblingRequest(t *testing.T) {
	finish := make(chan struct{})
	client, url, _ := pooledTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "first"); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-finish:
			if _, err := io.WriteString(w, "last"); err != nil {
				t.Error(err)
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := client.Do(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := client.Do(second)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	close(finish)
	if err := a.Body.Close(); err != nil {
		t.Log(err)
	}
	text, err := io.ReadAll(b.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(text) != "firstlast" {
		t.Fatalf("sibling response truncated: %q", text)
	}
}

func TestOrderedH2PoolResetDiscardsLateReturnedConnections(t *testing.T) {
	client, url, dials := pooledTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	})
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	client.CloseIdleConnections()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := client.Do(req.Clone(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(next.Body); err != nil {
		t.Fatal(err)
	}
	if err := next.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("late return repopulated a closed pool: %d", n)
	}
}
