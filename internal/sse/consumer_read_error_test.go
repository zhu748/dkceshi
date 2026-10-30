package sse

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failingSSEReader struct{ err error }

func (r failingSSEReader) Read([]byte) (int, error) { return 0, r.err }

func TestCollectStreamReportsReadErrorsBeforeCompletion(t *testing.T) {
	readErr := errors.New("connection reset by peer")
	for _, payload := range []string{"", "data: {\"v\":\"partial answer\"}\n"} {
		resp := &http.Response{Body: io.NopCloser(io.MultiReader(strings.NewReader(payload), failingSSEReader{readErr}))}
		got := CollectStream(resp, false, true)
		if !errors.Is(got.ReadError, readErr) {
			t.Fatalf("read error discarded after %q: %+v", payload, got)
		}
		if payload != "" && got.Text != "partial answer" {
			t.Fatalf("partial text must remain available for diagnostics: %q", got.Text)
		}
	}
}

func TestCollectStreamIgnoresReadErrorAfterTerminalStatus(t *testing.T) {
	for _, terminal := range []string{
		"data: [DONE]\n",
		"data: {\"p\":\"response/status\",\"v\":\"FINISHED\"}\n",
		"data: {\"p\":\"response/status\",\"v\":\"CONTENT_FILTER\"}\n",
	} {
		body := io.MultiReader(strings.NewReader("data: {\"v\":\"answer\"}\n"+terminal), failingSSEReader{io.ErrUnexpectedEOF})
		got := CollectStream(&http.Response{Body: io.NopCloser(body)}, false, true)
		if got.ReadError != nil || got.Text != "answer" {
			t.Fatalf("terminal status must take precedence: %+v", got)
		}
	}
}
