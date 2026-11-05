// proxybridge exposes one TURN or SSTP node as a loopback-only SOCKS5 proxy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
)

type config struct {
	Scheme     string `json:"scheme"`
	Server     string `json:"server"`
	Port       int    `json:"server_port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	ServerName string `json:"tls_server_name"`
	CAFile     string `json:"ca_file"`
	Insecure   bool   `json:"insecure"`
	Realm      string `json:"realm"`
	MTU        int    `json:"mtu"`
	ListenPort int    `json:"listen_port"`
	ReadyFile  string `json:"ready_file"`
}

type dialFunc func(context.Context, string) (net.Conn, error)

type protocolError struct {
	code  string
	cause error
}

func (e protocolError) Error() string {
	// The SSTP wrapper only carries its own diagnostics (assigned IP, packet
	// counters, dial error text); nothing peer-supplied flows in here, so the
	// cause is safe to surface. Returning the bare code discarded it.
	if e.cause == nil {
		return e.code
	}
	return e.code + ": " + e.cause.Error()
}
func (e protocolError) Unwrap() error { return e.cause }

var diagnostics atomic.Uint32

// Diagnostic messages are controlled codes: peer-provided errors can contain
// echoed credentials. Bound output even when an offline node is retried often.
func diagnostic(code string) {
	if diagnostics.Add(1) <= 128 {
		fmt.Fprintln(os.Stderr, code)
	}
}

func main() {
	path := flag.String("config", "", "path to private JSON configuration")
	watch := flag.Bool("watch-parent", false, "stop when the parent process exits")
	version := flag.Bool("version", false, "show supported protocols")
	flag.Parse()
	if *version {
		fmt.Println("ds2api-proxy-bridge 1 (TURN RFC6062; SSTP MS-CHAPv2)")
		return
	}
	// Dependencies use slog too; protocol diagnostics must never print secrets.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var parent io.Reader
	if *watch {
		parent = os.Stdin
	}
	if err := runWithParent(*path, parent); err != nil {
		diagnosticError(err)
		diagnostic(err.Error())
		os.Exit(1)
	}
}

func run(path string) error { return runWithParent(path, nil) }

func runWithParent(path string, parent io.Reader) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.New("config_read_failed")
	}
	var cfg config
	err = json.NewDecoder(io.LimitReader(f, 65536)).Decode(&cfg)
	closeBridge(f)
	if err != nil || cfg.Server == "" || cfg.Port < 1 || cfg.Port > 65535 ||
		cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return errors.New("invalid_config")
	}
	// SSTP authenticates with MS-CHAPv2, so it always needs credentials.
	// TURN/TURNS may omit them on open servers that accept unauthenticated
	// allocations; those use the anonymous dialer chosen below.
	if cfg.Scheme == "sstp" && (cfg.Username == "" || cfg.Password == "") {
		return errors.New("invalid_config")
	}
	if cfg.Scheme == "sstp" && cfg.MTU != 0 && (cfg.MTU < 576 || cfg.MTU > 9000) {
		return errors.New("invalid_config")
	}
	if cfg.ServerName == "" {
		cfg.ServerName = cfg.Server
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	if parent != nil {
		go func() {
			if _, err := io.Copy(io.Discard, parent); err != nil {
				diagnostic("parent_pipe_failed")
			}
			cancel()
		}()
	}
	// Reserve the SOCKS port before an expensive SSTP handshake. Otherwise a
	// different process can claim it during TLS/PPP negotiation, wasting a VPN
	// session and up to 30 seconds before we discover the bind failure.
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.ListenPort)))
	if err != nil {
		return errors.New("listen_failed")
	}
	defer closeBridge(listener)
	var dial dialFunc
	var closeTunnel func()
	switch cfg.Scheme {
	case "turn", "turns":
		tlsConfig, err := cfg.tlsConfig()
		if err != nil {
			return err
		}
		// One probe decides the client flavor: open servers that accept
		// unauthenticated allocations break the Pion client (it expects
		// a 401 challenge first), so they get the raw dialer instead.
		mode, err := detectTURNMode(ctx, cfg, tlsConfig)
		if err != nil {
			return err
		}
		dial = func(ctx context.Context, target string) (net.Conn, error) {
			return dialResolved(ctx, target, false, func(ctx context.Context, addr *net.TCPAddr) (net.Conn, error) {
				if mode == "anonymous" {
					return cfg.dialTURNAnonymous(ctx, addr, tlsConfig)
				}
				return cfg.dialTURN(ctx, addr, tlsConfig)
			})
		}
		closeTunnel = func() {}
	case "sstp":
		dial, closeTunnel, err = cfg.startSSTP(ctx, cancel)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported_protocol")
	}
	defer closeTunnel()
	if cfg.ReadyFile != "" {
		if err := os.WriteFile(cfg.ReadyFile, []byte("ready\n"), 0600); err != nil {
			closeBridge(listener)
			return errors.New("ready_write_failed")
		}
		defer os.Remove(cfg.ReadyFile)
	}
	return serveSOCKS(ctx, listener, dial)
}

// Resolve locally and pass only numeric targets to protocol clients. The helper
// never uses the ordinary TCP dialer for a target connection.
