package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestAnonymousTURNExplicitCancellationInterruptsRead(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer closeBridge(ln)
	reading := make(chan struct{})
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer closeBridge(c)
		b := make([]byte, 20)
		if _, err := io.ReadFull(c, b); err == nil {
			close(reading)
			_, _ = io.Copy(io.Discard, c)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cfg := config{Scheme: "turn", Server: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
	done := make(chan error, 1)
	go func() {
		c, err := cfg.dialTURNAnonymous(ctx, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}, nil)
		if c != nil {
			closeBridge(c)
		}
		done <- err
	}()
	select {
	case <-reading:
	case <-time.After(time.Second):
		t.Fatal("no Allocate")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("explicit cancel waited for socket deadline")
	}
}
