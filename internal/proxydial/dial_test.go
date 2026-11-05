package proxydial

import (
	"bufio"
	"context"
	"crypto/x509"
	"ds2api/internal/config"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, serve func(net.Conn)) config.Proxy {
	t.Helper()
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := ln.Close(); e != nil {
			t.Log(e)
		}
	})
	go func() {
		c, e := ln.Accept()
		if e == nil {
			defer closeConn(c)
			serve(c)
		}
	}()
	return config.Proxy{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
}
func TestSOCKS4WireAndRemoteDNS(t *testing.T) {
	for _, kind := range []string{"socks4", "socks4a"} {
		t.Run(kind, func(t *testing.T) {
			got := make(chan []byte, 1)
			p := fixture(t, func(c net.Conn) {
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				br := bufio.NewReader(c)
				head := make([]byte, 8)
				if _, e := io.ReadFull(br, head); e != nil {
					return
				}
				user, e := br.ReadBytes(0)
				if e != nil {
					return
				}
				head = append(head, user...)
				if kind == "socks4a" {
					host, e := br.ReadBytes(0)
					if e != nil {
						return
					}
					head = append(head, host...)
				}
				got <- head
				if _, e := c.Write([]byte{0, 90, 0, 0, 0, 0, 0, 0}); e != nil {
					return
				}
				b := make([]byte, 4)
				if _, e := io.ReadFull(br, b); e == nil {
					if _, e = c.Write(b); e != nil {
						return
					}
				}
			})
			p.Type, p.Username = kind, "userid"
			dial, e := New(p)
			if e != nil {
				t.Fatal(e)
			}
			target := "127.0.0.2:443"
			if kind == "socks4a" {
				target = "unresolvable.invalid:443"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c, e := dial(ctx, "tcp", target)
			if e != nil {
				t.Fatal(e)
			}
			defer closeConn(c)
			wire := <-got
			if wire[0] != 4 || wire[1] != 1 || binary.BigEndian.Uint16(wire[2:4]) != 443 || string(wire[8:15]) != "userid\x00" {
				t.Fatalf("bad wire: %v", wire)
			}
			if kind == "socks4a" && (!strings.HasSuffix(string(wire), "unresolvable.invalid\x00") || wire[7] != 1) {
				t.Fatal("remote DNS target lost")
			}
			if _, e = c.Write([]byte("ping")); e != nil {
				t.Fatal(e)
			}
			b := make([]byte, 4)
			if _, e = io.ReadFull(c, b); e != nil || string(b) != "ping" {
				t.Fatalf("tunnel failed %q %v", b, e)
			}
		})
	}
}
func TestHTTPConnectPreservesBufferedBytesAndAuthentication(t *testing.T) {
	auth := make(chan string, 1)
	p := fixture(t, func(c net.Conn) {
		req, e := http.ReadRequest(bufio.NewReader(c))
		if e != nil {
			return
		}
		auth <- req.Header.Get("Proxy-Authorization")
		if req.Method != "CONNECT" || req.Host != "target.example:443" {
			return
		}
		if _, e = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\nhello")); e != nil {
			return
		}
	})
	p.Type, p.Username, p.Password = "http", "user", "pass"
	d, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, e := d(ctx, "tcp", "target.example:443")
	if e != nil {
		t.Fatal(e)
	}
	defer closeConn(c)
	b := make([]byte, 5)
	if _, e = io.ReadFull(c, b); e != nil || string(b) != "hello" {
		t.Fatalf("buffer lost %q %v", b, e)
	}
	if <-auth != "Basic dXNlcjpwYXNz" {
		t.Fatal("authentication missing")
	}
}
func TestHTTPSProxyCertificateValidation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" {
			w.WriteHeader(400)
			return
		}
		h, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		c, b, e := h.Hijack()
		if e != nil {
			return
		}
		defer closeConn(c)
		if _, e = b.WriteString("HTTP/1.1 200 OK\r\n\r\nsecure"); e != nil {
			return
		}
		if e = b.Flush(); e != nil {
			return
		}
	}))
	defer server.Close()
	addr := server.Listener.Addr().(*net.TCPAddr)
	p := config.Proxy{Type: "https", Host: "127.0.0.1", Port: addr.Port}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	if c, e := d(ctx, "tcp", "target.example:443"); e == nil {
		closeConn(c)
		t.Fatal("untrusted proxy accepted")
	}
	cert, e := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if e != nil {
		t.Fatal(e)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if e = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	p.CAFile = ca
	d, e = New(p)
	if e != nil {
		t.Fatal(e)
	}
	c, e := d(ctx, "tcp", "target.example:443")
	if e != nil {
		t.Fatal(e)
	}
	defer closeConn(c)
	b := make([]byte, 6)
	if _, e = io.ReadFull(c, b); e != nil || string(b) != "secure" {
		t.Fatalf("secure tunnel %q %v", b, e)
	}
}
func TestHandshakeCancellation(t *testing.T) {
	p := fixture(t, func(c net.Conn) {
		b := make([]byte, 64)
		for {
			if _, e := c.Read(b); e != nil {
				return
			}
		}
	})
	p.Type = "http"
	d, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	start := time.Now()
	if c, e := d(ctx, "tcp", "target.example:443"); e == nil {
		closeConn(c)
		t.Fatal("stalled proxy accepted")
	}
	if time.Since(start) > time.Second {
		t.Fatal("request cancellation ignored")
	}
}

func TestConnectHeadersBoundedWithoutTruncatingTunnel(t *testing.T) {
	for _, large := range []bool{true, false} {
		t.Run(fmt.Sprintf("large=%v", large), func(t *testing.T) {
			p := fixture(t, func(c net.Conn) {
				if _, err := http.ReadRequest(bufio.NewReader(c)); err != nil {
					return
				}
				response := "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\n\r\n" + strings.Repeat("x", 2*maxProxyHeaders)
				if large {
					response = "HTTP/1.1 200 OK\r\nX-Large: " + strings.Repeat("x", 2*maxProxyHeaders) + "\r\n\r\n"
				}
				if _, err := io.WriteString(c, response); err != nil {
					return
				}
			})
			p.Type = "http"
			d, err := New(p)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := d(ctx, "tcp", "target.example:443")
			if large {
				if err == nil {
					closeConn(conn)
					t.Fatal("unbounded header accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closeConn(conn)
			payload, err := io.ReadAll(conn)
			if err != nil || len(payload) != 2*maxProxyHeaders {
				t.Fatalf("tunnel truncated: %d %v", len(payload), err)
			}
		})
	}
}
func TestCanceledHandshakesPreserveContextError(t *testing.T) {
	for _, kind := range []string{"http", "socks4", "socks5h"} {
		t.Run(kind, func(t *testing.T) {
			p := fixture(t, func(c net.Conn) {
				if _, err := io.Copy(io.Discard, c); err != nil {
					return
				}
			})
			p.Type = kind
			d, err := New(p)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			_, err = d(ctx, "tcp", "127.0.0.1:443")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancellation lost context identity: %v", err)
			}
		})
	}
}
