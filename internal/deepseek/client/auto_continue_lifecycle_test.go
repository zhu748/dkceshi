package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAutoContinueCloseUnblocksUnderlyingBody(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	body := newAutoContinueBody(context.Background(), reader, "session", 1, func(context.Context, string, int) (*http.Response, error) {
		t.Error("unexpected continuation")
		return nil, io.ErrUnexpectedEOF
	})
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := writer.Write([]byte("blocked")); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("upstream read survived closing the consumer")
		}
	case <-time.After(time.Second):
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
		t.Fatal("upstream reader leaked")
	}
}

func TestAutoContinueLimitDoesNotReportSuccess(t *testing.T) {
	raw := "data: {\"response_message_id\":7,\"p\":\"response/status\",\"v\":\"INCOMPLETE\"}\ndata: [DONE]\n"
	body := newAutoContinueBody(context.Background(), io.NopCloser(strings.NewReader(raw)), "session", 1, func(context.Context, string, int) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	defer func() {
		if err := body.Close(); err != nil {
			t.Error(err)
		}
	}()
	_, err := io.ReadAll(body)
	if err == nil || !strings.Contains(err.Error(), "continuation limit") {
		t.Fatalf("incomplete message reported as successful: %v", err)
	}
}

func TestAutoContinueDoneDoesNotWaitForConnectionEOF(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	body := newAutoContinueBody(context.Background(), reader, "session", 1, func(context.Context, string, int) (*http.Response, error) {
		t.Error("unexpected continuation")
		return nil, io.ErrUnexpectedEOF
	})
	defer func() {
		if err := body.Close(); err != nil {
			t.Error(err)
		}
	}()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(body); done <- err }()
	if _, err := io.WriteString(writer, "data: [DONE]\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("[DONE] waited for upstream TCP EOF")
	}
}
