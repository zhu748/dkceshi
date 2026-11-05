package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failedStreamBody struct{}

func (failedStreamBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedStreamBody) Close() error             { return nil }

func TestProxyReportsStreamOutcomeOnceAndIgnoresCancellation(t *testing.T) {
	for _, scenario := range []string{"interrupted", "complete", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", "https://target.invalid", nil)
			var reports []bool
			var body io.ReadCloser = failedStreamBody{}
			if scenario == "complete" {
				body = io.NopCloser(strings.NewReader("data"))
			}
			d := reportingDoer{inner: alignmentDoer(func(*http.Request) (*http.Response, error) { return &http.Response{Body: body}, nil }), id: "p", report: func(_ string, success bool) { reports = append(reports, success) }}
			resp, err := d.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if len(reports) != 0 {
				t.Fatal("reported success before reading the stream")
			}
			if scenario == "cancelled" {
				cancel()
			}
			_, readErr := io.ReadAll(resp.Body)
			if scenario != "complete" && !errors.Is(readErr, io.ErrUnexpectedEOF) {
				t.Fatal(readErr)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if scenario == "cancelled" {
				if len(reports) != 0 {
					t.Fatal(reports)
				}
				return
			}
			if len(reports) != 1 || reports[0] != (scenario == "complete") {
				t.Fatalf("reports %v", reports)
			}
		})
	}
}
