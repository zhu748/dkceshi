package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParentEOFStopsRunningHelper(t *testing.T) {
	cfg := turnFixture(t, false)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ListenPort = listener.Addr().(*net.TCPAddr).Port
	closeBridge(listener)
	dir := t.TempDir()
	cfg.ReadyFile = filepath.Join(dir, "ready")
	path := filepath.Join(dir, "config.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reader, parent := io.Pipe()
	defer closeBridge(reader)
	defer closeBridge(parent)
	done := make(chan error, 1)
	go func() { done <- runWithParent(path, reader) }()
	deadline := time.Now().Add(4 * time.Second)
	for {
		if _, err = os.Stat(cfg.ReadyFile); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("helper exited before readiness: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	closeBridge(parent)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper failed during parent EOF cleanup: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("helper survived parent EOF")
	}
}
