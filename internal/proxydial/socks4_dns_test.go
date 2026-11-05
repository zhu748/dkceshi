package proxydial

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestSOCKS4RetriesIPv4CandidatesWithinOneBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var attempts []string
	_, err := dialSOCKS5(ctx, "socks4", "tcp", "example.com:443", func(_ context.Context, family, _ string) ([]string, error) {
		if family != "ip4" {
			t.Fatalf("family %s", family)
		}
		return []string{"2001:db8::1", "203.0.113.1", "203.0.113.2"}, nil
	}, func(attempt context.Context, network, target string) (net.Conn, error) {
		if network != "tcp4" {
			t.Fatalf("network %s", network)
		}
		attempts = append(attempts, target)
		if len(attempts) == 1 {
			deadline, _ := attempt.Deadline()
			full, _ := ctx.Deadline()
			if !deadline.Before(full) {
				t.Fatal("no budget reserved for the second address")
			}
		}
		return nil, errors.New("fixture refusal")
	})
	if err == nil || len(attempts) != 2 || attempts[1] != "203.0.113.2:443" {
		t.Fatalf("attempts %v, error %v", attempts, err)
	}
}
