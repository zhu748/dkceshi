package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
)

func TestInspectionClosesBlockedFirstAndJSONReads(t *testing.T) {
	for _, prefix := range []string{"", `{"data":`} {
		t.Run(prefix, func(t *testing.T) {
			t.Setenv("DS2API_STREAM_IDLE_TIMEOUT_MS", "20")
			reader, writer := io.Pipe()
			t.Cleanup(func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			})
			if prefix != "" {
				go func() { _, _ = writer.Write([]byte(prefix)) }()
			}
			response := &http.Response{StatusCode: 200, Body: reader}
			done := make(chan error, 1)
			go func() {
				done <- (&Client{}).inspectCompletionFailureContext(context.Background(), &auth.RequestAuth{}, response, "test")
			}()
			select {
			case err := <-done:
				if !errors.Is(err, errInspectionIdle) {
					t.Fatalf("lost timeout cause: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("blocked inspection ignored idle timeout")
			}
		})
	}
}

func TestInspectionHonorsCancellationAndTotalBudget(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{true: "cancelled", false: "total"}[cancelled], func(t *testing.T) {
			t.Setenv("DS2API_STREAM_IDLE_TIMEOUT_MS", "200")
			t.Setenv("DS2API_STREAM_MAX_DURATION_MS", "30")
			reader, writer := io.Pipe()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			err := (&Client{}).inspectCompletionFailureContext(ctx, &auth.RequestAuth{}, &http.Response{StatusCode: 200, Body: reader}, "test")
			if err == nil || cancelled && !errors.Is(err, context.Canceled) || !cancelled && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("wrong failure: %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInspectionReplaysPrefixAndRetiresItsTimer(t *testing.T) {
	t.Setenv("DS2API_STREAM_IDLE_TIMEOUT_MS", "10")
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n"))}
	if err := (&Client{}).inspectCompletionFailureContext(context.Background(), &auth.RequestAuth{}, response, "test"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "data: [DONE]\n\n" {
		t.Fatalf("inspection consumed or closed SSE: %q %v", data, err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}
