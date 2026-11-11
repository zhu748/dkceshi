package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type observedContinueBody struct {
	*io.PipeReader
	reading, closed chan struct{}
	readOnce        sync.Once
	closes          atomic.Int32
	closeErr        error
}

func (b *observedContinueBody) Read(p []byte) (int, error) {
	b.readOnce.Do(func() { close(b.reading) })
	return b.PipeReader.Read(p)
}

func (b *observedContinueBody) Close() error {
	if b.closes.Add(1) == 1 {
		close(b.closed)
	}
	return errors.Join(b.PipeReader.Close(), b.closeErr)
}

func TestAutoContinueCloseReturnsUpstreamErrorOnce(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	}()
	want := errors.New("upstream close failed")
	upstream := &observedContinueBody{PipeReader: reader, reading: make(chan struct{}), closed: make(chan struct{}), closeErr: want}
	body := newAutoContinueBody(context.Background(), upstream, "session", 1, func(context.Context, string, int) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	for range 2 {
		if err := body.Close(); !errors.Is(err, want) {
			t.Fatalf("lost upstream cleanup failure: %v", err)
		}
	}
	if got := upstream.closes.Load(); got != 1 {
		t.Fatalf("upstream closed %d times", got)
	}
}

func TestAutoContinueCloseOwnsActiveAndLateContinuation(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "late"}[late], func(t *testing.T) {
			reader, writer := io.Pipe()
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			upstream := &observedContinueBody{PipeReader: reader, reading: make(chan struct{}), closed: make(chan struct{})}
			opened, release := make(chan struct{}), make(chan struct{})
			if !late {
				close(release)
			}
			initial := io.NopCloser(strings.NewReader("data: {\"response_message_id\":7,\"p\":\"response/status\",\"v\":\"INCOMPLETE\"}\ndata: [DONE]\n"))
			body := newAutoContinueBody(context.Background(), initial, "session", 1, func(context.Context, string, int) (*http.Response, error) {
				close(opened)
				<-release // Emulate an opener that returns after its context was cancelled.
				return &http.Response{StatusCode: 200, Body: upstream}, nil
			})
			consumed := make(chan struct{})
			go func() {
				_, err := io.Copy(io.Discard, body)
				if err != nil && !errors.Is(err, io.ErrClosedPipe) {
					t.Error(err)
				}
				close(consumed)
			}()
			select {
			case <-opened:
			case <-time.After(time.Second):
				t.Fatal("continuation did not open")
			}
			if !late {
				select {
				case <-upstream.reading:
				case <-time.After(time.Second):
					t.Fatal("continuation did not begin reading")
				}
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			if late {
				close(release)
			}
			select {
			case <-upstream.closed:
			case <-time.After(time.Second):
				t.Fatal("upstream body leaked")
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-consumed:
			case <-time.After(time.Second):
				t.Fatal("downstream read survived close")
			}
			if got := upstream.closes.Load(); got != 1 {
				t.Fatalf("upstream closed %d times", got)
			}
		})
	}
}
