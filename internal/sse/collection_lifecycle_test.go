package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCollectStreamRejectsEOFWithoutCompletion(t *testing.T) {
	for _, body := range []string{"", "data: {\"v\":\"partial answer\"}\n"} {
		result := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(body))}, false, true)
		if !errors.Is(result.ReadError, ErrUnexpectedStreamEOF) {
			t.Fatalf("missing EOF failure: %+v", result)
		}
	}
}

func TestTerminalBatchKeepsItsLastContent(t *testing.T) {
	for _, status := range []string{"status", "quasi_status"} {
		raw := `data: {"p":"response","v":[{"p":"content","v":"final tail"},{"p":"` + status + `","v":"FINISHED"}]}` + "\n"
		result := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(raw))}, false, true)
		if result.ReadError != nil || result.Text != "final tail" {
			t.Fatalf("terminal batch lost output: %+v", result)
		}
	}
	raw := `data: {"v":{"response":{"status":"FINISHED","fragments":[{"type":"RESPONSE","content":"snapshot tail"}]}}}` + "\n"
	result := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(raw))}, false, true)
	if result.ReadError != nil || result.Text != "snapshot tail" {
		t.Fatalf("terminal snapshot lost output: %+v", result)
	}
	raw = `data: {"v":{"status":"FINISHED","fragments":[{"type":"RESPONSE","content":"bare snapshot"}]}}` + "\n"
	result = CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(raw))}, false, true)
	if result.ReadError != nil || result.Text != "bare snapshot" {
		t.Fatalf("bare snapshot lost output: %+v", result)
	}
}

func TestCollectionDeadlinesCloseBlockedBody(t *testing.T) {
	for _, mode := range []string{"idle", "total", "cancel", "finished"} {
		t.Run(mode, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := CollectOptions{Context: ctx, IdleTimeout: time.Second, MaxDuration: time.Second, TerminalDrainTimeout: 20 * time.Millisecond}
			if mode == "idle" {
				opts.IdleTimeout = 20 * time.Millisecond
			}
			if mode == "total" {
				opts.MaxDuration = 20 * time.Millisecond
			}
			done := make(chan CollectResult, 1)
			go func() { done <- CollectStreamWithOptions(&http.Response{Body: reader}, false, false, opts) }()
			if mode == "finished" {
				_, err := io.WriteString(writer, "data: {\"v\":\"complete\"}\ndata: {\"p\":\"response/status\",\"v\":\"FINISHED\"}\n")
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case result := <-done:
				if mode == "finished" {
					if result.ReadError != nil || result.Text != "complete" {
						t.Fatalf("lost completed answer: %+v", result)
					}
				} else if result.ReadError == nil {
					t.Fatalf("deadline was swallowed: %+v", result)
				}
				if mode == "cancel" && !errors.Is(result.ReadError, context.Canceled) {
					t.Fatalf("lost cancellation: %v", result.ReadError)
				}
			case <-time.After(time.Second):
				if err := reader.Close(); err != nil {
					t.Error(err)
				}
				t.Fatal("blocked upstream body survived deadline")
			}
		})
	}
}
