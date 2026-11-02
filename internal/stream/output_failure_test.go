package stream

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestKeepaliveWriteFailureStopsASilentUpstream(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var outputErr error
	var stopped StopReason
	ConsumeSSE(ConsumeConfig{Context: ctx, Body: reader, KeepAliveInterval: time.Millisecond}, ConsumeHooks{
		OnKeepAlive: func() { outputErr = errors.New("broken response") },
		OutputError: func() error { return outputErr },
		OnFinalize: func(reason StopReason, err error) {
			stopped = reason
			if err != nil {
				t.Error(err)
			}
		},
		OnContextDone: func() { t.Error("waited for the upstream after output failed") },
	})
	if stopped != "client_write_error" {
		t.Fatalf("stop reason=%q", stopped)
	}
}
