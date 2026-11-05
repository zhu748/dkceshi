package proxybridge

import (
	"bufio"
	"context"
	"ds2api/internal/config"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBridgeFixture(t *testing.T) {
	if os.Getenv("DS2API_BRIDGE_TEST_CHILD") != "1" {
		return
	}
	var path string
	for i, arg := range os.Args {
		if arg == "-config" && i+1 < len(os.Args) {
			path = os.Args[i+1]
		}
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var cfg struct {
		Port  int    `json:"listen_port"`
		Ready string `json:"ready_file"`
	}
	if e = json.Unmarshal(data, &cfg); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(os.Getenv("DS2API_BRIDGE_TEST_CONFIG"), []byte(path), 0600); e != nil {
		t.Fatal(e)
	}
	ln, e := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port)))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Log(err)
		}
	}()
	if os.Getenv("DS2API_BRIDGE_TEST_STALL") == "1" {
		c, err := ln.Accept()
		if err == nil {
			fixtureClose(c)
		}
		return
	}
	if e = os.WriteFile(cfg.Ready, []byte("ready"), 0600); e != nil {
		t.Fatal(e)
	}
	for {
		conn, e := ln.Accept()
		if e != nil {
			return
		}
		go func(c net.Conn) {
			defer fixtureClose(c)
			br := bufio.NewReader(c)
			head := make([]byte, 2)
			if _, e := io.ReadFull(br, head); e != nil {
				return
			}
			methods := make([]byte, int(head[1]))
			if _, e = io.ReadFull(br, methods); e != nil {
				return
			}
			if _, e = c.Write([]byte{5, 0}); e != nil {
				return
			}
			req := make([]byte, 4)
			if _, e = io.ReadFull(br, req); e != nil {
				return
			}
			size := 0
			switch req[3] {
			case 1:
				size = 4
			case 4:
				size = 16
			case 3:
				b, e := br.ReadByte()
				if e != nil {
					return
				}
				size = int(b)
			default:
				return
			}
			if _, e = io.CopyN(io.Discard, br, int64(size+2)); e != nil {
				return
			}
			if _, e = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); e != nil {
				return
			}
			if _, e = io.Copy(c, br); e != nil {
				return
			}
		}(conn)
	}
}
func fixtureClose(c net.Conn) {
	if e := c.Close(); e != nil {
		config.Logger.Debug("test fixture cleanup", "error", e)
	}
}
func helperFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX launch script")
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "helper")
	quoted := "'" + strings.ReplaceAll(exe, "'", "'\"'\"'") + "'"
	if e = os.WriteFile(script, []byte("#!/bin/sh\nexec "+quoted+" -test.run='^TestBridgeFixture$' -- \"$@\"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(dir, "config-path")
	t.Setenv("DS2API_BRIDGE_TEST_CHILD", "1")
	t.Setenv("DS2API_BRIDGE_TEST_CONFIG", marker)
	t.Setenv("DS2API_PROXY_BRIDGE_BIN", script)
	t.Cleanup(CloseAll)
	return marker
}
func TestHelperReusePrivateConfigAndRestart(t *testing.T) {
	marker := helperFixture(t)
	p := config.Proxy{Type: "turn", Host: "relay.example", Port: 3478, Username: "u", Password: "private"}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, e := Dial(ctx, p, "tcp", "target.invalid:443")
	if e != nil {
		t.Fatal(e)
	}
	configPath, e := os.ReadFile(marker)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(string(configPath)); !os.IsNotExist(e) {
		t.Fatal("private configuration not removed after startup")
	}
	if _, e = c.Write([]byte("ping")); e != nil {
		t.Fatal(e)
	}
	b := make([]byte, 4)
	if _, e = io.ReadFull(c, b); e != nil || string(b) != "ping" {
		t.Fatalf("forwarding failed: %q %v", b, e)
	}
	p.Name = "renamed"
	second, e := Dial(ctx, p, "tcp", "target.invalid:443")
	if e != nil {
		t.Fatal(e)
	}
	registry.Lock()
	entry := registry.items[config.ProxyIdentity(p)]
	count := len(registry.items)
	registry.Unlock()
	if count != 1 || entry.refs.Load() != 2 {
		t.Fatal("connection settings did not reuse helper")
	}
	fixtureClose(c)
	fixtureClose(second)
	if entry.refs.Load() != 0 {
		t.Fatal("active leases leaked")
	}
	entry.mu.Lock()
	pid := entry.cmd.Process.Pid
	entry.stop()
	entry.mu.Unlock()
	third, e := Dial(ctx, p, "tcp", "target.invalid:443")
	if e != nil {
		t.Fatal(e)
	}
	fixtureClose(third)
	entry.mu.Lock()
	newPID := entry.cmd.Process.Pid
	entry.mu.Unlock()
	if newPID == pid {
		t.Fatal("dead helper not restarted")
	}
}
func TestStartupAndLockWaitCancellation(t *testing.T) {
	helperFixture(t)
	t.Setenv("DS2API_BRIDGE_TEST_STALL", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := Dial(ctx, config.Proxy{Type: "sstp", Host: "vpn.example", Port: 443, Username: "u", Password: "p"}, "tcp", "target.invalid:443")
	if e == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("startup cancellation failed: %v", e)
	}
	instance := &instance{}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if e = instance.lockContext(ctx2); e == nil {
		t.Fatal("canceled mutex wait succeeded")
	}
	registry.Lock()
	defer registry.Unlock()
	for _, entry := range registry.items {
		if entry.refs.Load() != 0 || entry.cmd != nil {
			t.Fatal("startup leaked process or reference")
		}
	}
}

func TestFailedStartsDoNotConsumeRegistryCapacity(t *testing.T) {
	t.Setenv("DS2API_PROXY_BRIDGE_BIN", filepath.Join(t.TempDir(), "missing-helper"))
	t.Cleanup(CloseAll)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < 140; i++ {
		p := config.Proxy{Type: "turn", Host: "relay" + strconv.Itoa(i) + ".example", Port: 3478}
		if conn, err := Dial(ctx, p, "tcp", "target.invalid:443"); err == nil {
			fixtureClose(conn)
			t.Fatal("missing helper unexpectedly started")
		}
	}
	registry.Lock()
	defer registry.Unlock()
	if len(registry.items) != 0 {
		t.Fatalf("failed helpers consumed %d registry slots", len(registry.items))
	}
}

func TestShutdownCancelsPendingStartup(t *testing.T) {
	marker := helperFixture(t)
	t.Setenv("DS2API_BRIDGE_TEST_STALL", "1")
	result := make(chan error, 1)
	go func() {
		_, err := Dial(context.Background(), config.Proxy{Type: "sstp", Host: "vpn.example", Port: 443, Username: "u", Password: "p"}, "tcp", "target.invalid:443")
		result <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	CloseAll()
	if time.Since(start) > time.Second {
		t.Fatal("shutdown waited for the VPN handshake timeout")
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup cancellation lost context: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup survived shutdown")
	}
}

func TestIdleReaperKeepsActiveTunnels(t *testing.T) {
	helperFixture(t)
	p := config.Proxy{Type: "turn", Host: "relay.example", Port: 3478}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := Dial(ctx, p, "tcp", "target.invalid:443")
	if err != nil {
		t.Fatal(err)
	}
	key := config.ProxyIdentity(p)
	registry.Lock()
	entry := registry.items[key]
	registry.Unlock()
	entry.last.Store(time.Now().Add(-time.Hour).UnixNano())
	reapIdle(time.Now())
	registry.Lock()
	kept := registry.items[key] == entry
	registry.Unlock()
	if !kept {
		t.Fatal("reaper removed an active tunnel")
	}
	fixtureClose(conn)
	entry.last.Store(time.Now().Add(-time.Hour).UnixNano())
	reapIdle(time.Now())
	registry.Lock()
	kept = registry.items[key] != nil
	registry.Unlock()
	if kept {
		t.Fatal("idle tunnel was not reclaimed")
	}
}

func TestCapacityEvictsOldestIdleHelperAndProtectsActiveOnes(t *testing.T) {
	helperFixture(t)
	registry.Lock()
	for i := 0; i < 128; i++ {
		life, cancel := context.WithCancel(context.Background())
		entry := &instance{lifetime: life, cancel: cancel}
		entry.last.Store(int64(i + 1))
		if i == 0 {
			entry.refs.Store(1)
		}
		registry.items["cached-"+strconv.Itoa(i)] = entry
	}
	registry.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := Dial(ctx, config.Proxy{Type: "turn", Host: "new.example", Port: 3478}, "tcp", "target.invalid:443")
	if err != nil {
		t.Fatal(err)
	}
	fixtureClose(conn)
	registry.Lock()
	defer registry.Unlock()
	if registry.items["cached-0"] == nil || registry.items["cached-1"] != nil || len(registry.items) != 128 {
		t.Fatal("capacity eviction removed an active helper or kept the oldest idle helper")
	}
}
