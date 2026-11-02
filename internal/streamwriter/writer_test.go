package streamwriter

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type testResponse struct {
	writes             int
	data               string
	writeErr, flushErr error
	short              bool
	deadlines          []time.Time
}

func (*testResponse) Header() http.Header { return http.Header{} }
func (*testResponse) WriteHeader(int)     {}
func (r *testResponse) Write(data []byte) (int, error) {
	r.writes++
	r.data += string(data)
	if r.writeErr != nil {
		return 0, r.writeErr
	}
	if r.short {
		return len(data) - 1, nil
	}
	return len(data), nil
}
func (r *testResponse) FlushError() error { return r.flushErr }
func (r *testResponse) SetWriteDeadline(at time.Time) error {
	r.deadlines = append(r.deadlines, at)
	return nil
}

func TestJSONIsOneFrameAndWriteDeadlineIsCleared(t *testing.T) {
	r := &testResponse{}
	var out Writer
	if !out.JSON(r, nil, true, "delta", map[string]any{"text": "中文"}) {
		t.Fatal(out.Err())
	}
	if r.writes != 1 || r.data != "event: delta\ndata: {\"text\":\"中文\"}\n\n" {
		t.Fatalf("invalid frame: %+v", r)
	}
	if len(r.deadlines) != 2 || r.deadlines[0].Before(time.Now()) || !r.deadlines[1].IsZero() {
		t.Fatal("deadline not bounded/cleared")
	}
}

func TestTransportFailuresAreSticky(t *testing.T) {
	boom := errors.New("broken socket")
	for _, r := range []*testResponse{{writeErr: boom}, {flushErr: boom}, {short: true}} {
		var out Writer
		if out.Write(r, nil, true, []byte("frame")) || out.Err() == nil {
			t.Fatal("failure ignored")
		}
		if r.short && !errors.Is(out.Err(), io.ErrShortWrite) {
			t.Fatal(out.Err())
		}
		if out.JSON(r, nil, true, "", map[string]any{}) || r.writes != 1 {
			t.Fatal("failed socket reused")
		}
		if !r.deadlines[len(r.deadlines)-1].IsZero() {
			t.Fatal("deadline leaked after error")
		}
	}
}

func TestSerializationFailureNeverWrites(t *testing.T) {
	r := &testResponse{}
	var out Writer
	if out.JSON(r, nil, true, "", map[string]any{"bad": make(chan int)}) || out.Err() == nil || r.writes != 0 {
		t.Fatal("invalid JSON delivered")
	}
	if !strings.Contains(out.Err().Error(), "unsupported type") {
		t.Fatal(out.Err())
	}
}
