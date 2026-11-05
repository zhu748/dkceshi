package proxydial

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestSOCKS5TriesRemainingLocalDNSAddresses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var attempted []string
	lookups := 0
	lookup := func(_ context.Context, network, host string) ([]string, error) {
		lookups++
		if network != "ip" || host != "example.com" {
			t.Fatal("unexpected DNS query")
		}
		return []string{"2001:db8::1", "203.0.113.10", "203.0.113.10"}, nil
	}
	var peer net.Conn
	dial := func(attempt context.Context, _ string, target string) (net.Conn, error) {
		attempted = append(attempted, target)
		if len(attempted) == 1 {
			a, _ := attempt.Deadline()
			b, _ := ctx.Deadline()
			if !a.Before(b) {
				t.Fatal("first address can exhaust the entire budget")
			}
			return nil, errors.New("IPv6 unavailable")
		}
		conn, other := net.Pipe()
		peer = other
		return conn, nil
	}
	conn, err := dialSOCKS5(ctx, "socks5", "tcp", "example.com:443", lookup, dial)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn(conn)
	defer closeConn(peer)
	if lookups != 1 || len(attempted) != 2 || attempted[0] != "[2001:db8::1]:443" || attempted[1] != "203.0.113.10:443" {
		t.Fatalf("remaining addresses not tried: %v", attempted)
	}
}
func TestSOCKS5hKeepsRemoteDNSAndIPTargetsSkipLookup(t *testing.T) {
	for _, tc := range []struct{ kind, target string }{{"socks5h", "example.invalid:443"}, {"socks5", "[2001:db8::1]:443"}} {
		lookups := 0
		var got string
		_, err := dialSOCKS5(context.Background(), tc.kind, "tcp", tc.target, func(context.Context, string, string) ([]string, error) {
			lookups++
			return nil, errors.New("unexpected lookup")
		}, func(_ context.Context, _ string, target string) (net.Conn, error) {
			got = target
			return nil, errors.New("fixture")
		})
		if err == nil || lookups != 0 || got != tc.target {
			t.Fatalf("target was resolved or changed: %s", got)
		}
	}
}
func TestSOCKS5CancellationStopsFurtherDNSCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	_, err := dialSOCKS5(ctx, "socks5", "tcp", "example.com:443", func(context.Context, string, string) ([]string, error) {
		return []string{"203.0.113.1", "203.0.113.2"}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		calls++
		cancel()
		return nil, errors.New("unavailable")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled lookup kept dialing: %d %v", calls, err)
	}
}
