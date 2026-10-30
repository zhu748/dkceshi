package stream

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"ds2api/internal/sse"
)

type blockingStreamBody struct {
	initial *strings.Reader
	closed  chan struct{}
	once    sync.Once
}

func (b *blockingStreamBody) Read(p []byte) (int, error) {
	if b.initial.Len() > 0 {
		return b.initial.Read(p)
	}
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *blockingStreamBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func TestConsumeSSEStopsReaderWhenConsumerFinishes(t *testing.T) {
	for _, initial := range []string{"", "data: {\"v\":\"visible answer\"}\n"} {
		t.Run(initial, func(t *testing.T) {
			body := &blockingStreamBody{initial: strings.NewReader(initial), closed: make(chan struct{})}
			t.Cleanup(func() {
				if err := body.Close(); err != nil {
					t.Error(err)
				}
			})
			parent := context.Background()
			var reason StopReason
			interval := 25 * time.Millisecond
			if initial != "" {
				interval = time.Second
			}
			ConsumeSSE(ConsumeConfig{
				Context: parent, Body: body, KeepAliveInterval: interval,
				MaxKeepAliveNoInput: 1,
			}, ConsumeHooks{
				OnParsed: func(parsed sse.LineResult) ParsedDecision {
					return ParsedDecision{Stop: len(parsed.Parts) > 0}
				},
				OnFinalize: func(stop StopReason, _ error) { reason = stop },
			})
			want := StopReasonNoContentTimeout
			if initial != "" {
				want = StopReasonHandlerRequested
			}
			if reason != want {
				t.Fatalf("unexpected stop: %s", reason)
			}
			if parent.Err() != nil {
				t.Fatal("finishing a stream must not cancel its caller")
			}
			select {
			case <-body.closed:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("consumer returned but upstream reader is still blocked")
			}
		})
	}
}
