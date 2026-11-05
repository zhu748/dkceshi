package transport

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEnvironmentConnectSharesHeaderLimitsAndInformationalResponses(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(map[bool]string{false: "interim", true: "large"}[large], func(t *testing.T) {
			client, server := net.Pipe()
			defer closeTransportConn(server)
			go func() {
				r, err := http.ReadRequest(bufio.NewReader(server))
				if err != nil {
					return
				}
				if r.Body != nil {
					if err := r.Body.Close(); err != nil {
						t.Log(err)
					}
				}
				reply := "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\n\r\npayload"
				if large {
					reply = "HTTP/1.1 200 OK\r\nX-Large: " + strings.Repeat("x", 70<<10) + "\r\n\r\n"
				}
				_, _ = io.WriteString(server, reply)
			}()
			p, _ := url.Parse("http://proxy.example:8080")
			conn, err := dialConnectTunnel(t.Context(), func(ctx context.Context, _, _ string) (net.Conn, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 16*time.Second {
					t.Error("CONNECT has no independent budget")
				}
				return client, nil
			}, p, "target.example:443")
			if large {
				if err == nil {
					closeTransportConn(conn)
					t.Fatal("unbounded header accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closeTransportConn(conn)
			b := make([]byte, 7)
			if _, err := io.ReadFull(conn, b); err != nil || string(b) != "payload" {
				t.Fatalf("tunnel bytes %q: %v", b, err)
			}
		})
	}
}

func TestTargetTLSHandshakeHasIndependentBudget(t *testing.T) {
	client, server := net.Pipe()
	defer closeTransportConn(server)
	go func() { _, _ = io.Copy(io.Discard, server) }()
	dial := fingerprintTLSDialer(func(context.Context, string, string) (net.Conn, error) { return client, nil }, nil)
	start := time.Now()
	conn, err := dial(t.Context(), "tcp", "target.example:443")
	if conn != nil {
		closeTransportConn(conn)
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 20*time.Second {
		t.Fatalf("TLS handshake was not bounded: %v", err)
	}
}
