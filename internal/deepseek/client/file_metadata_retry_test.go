package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCachedHistoryFileRetriesMetadataRateLimitWithoutReupload(t *testing.T) {
	for _, recover := range []bool{false, true} {
		c, upstream, a, req := newReuseTestClient()
		req.Filename, req.ContentType = "HISTORY.txt", "text/plain"
		first, err := c.UploadFile(context.Background(), a, req, 3)
		if err != nil {
			t.Fatal(err)
		}
		queries := 0
		c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/fetch_files") {
				queries++
				if r.Header.Get("Authorization") != "Bearer credential" {
					t.Fatal("changed file owner")
				}
				if !recover || queries < 3 {
					return &http.Response{StatusCode: 200, Header: http.Header{"Retry-After": []string{"5"}}, Body: io.NopCloser(strings.NewReader(`{"code":40029,"msg":"TOO_MANY_REQUESTS"}`))}, nil
				}
			}
			return upstream.Do(r)
		})
		original := fileReadySleep
		var delays []time.Duration
		fileReadySleep = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
		second, err := c.UploadFile(context.Background(), a, req, 3)
		fileReadySleep = original
		if queries != 3 || upstream.uploads != 1 || upstream.proofs != 1 || !reflect.DeepEqual(delays, []time.Duration{5 * time.Second, 5 * time.Second}) {
			t.Fatalf("queries=%d uploads=%d proofs=%d delays=%v", queries, upstream.uploads, upstream.proofs, delays)
		}
		if recover {
			if err != nil || second.ID != first.ID {
				t.Fatalf("cache retry failed: %#v %v", second, err)
			}
		} else {
			var limited *FileRateLimitError
			if !errors.As(err, &limited) {
				t.Fatalf("lost rate limit error: %v", err)
			}
			// A rejected query must leave the old file available for a later retry.
			c.regular = upstream
			third, err := c.UploadFile(context.Background(), a, req, 3)
			if err != nil || third.ID != first.ID || upstream.uploads != 1 {
				t.Fatalf("cache discarded after rate limit: %#v %v", third, err)
			}
		}
	}
}

func TestFileMetadataRetryCancellationStopsQueries(t *testing.T) {
	c, _, a, _ := newReuseTestClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queries := 0
	c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
		queries++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":40029}`))}, nil
	})
	original := fileReadySleep
	fileReadySleep = func(ctx context.Context, delay time.Duration) error { cancel(); return ctx.Err() }
	t.Cleanup(func() { fileReadySleep = original })
	_, err := c.FetchUploadedFiles(ctx, a, []string{"file-1"})
	if !errors.Is(err, context.Canceled) || queries != 1 {
		t.Fatalf("queries=%d err=%v", queries, err)
	}
}

func TestReadinessRateLimitDoesNotMultiplyRetryBudget(t *testing.T) {
	c, _, a, _ := newReuseTestClient()
	queries := 0
	c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
		queries++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":40029}`))}, nil
	})
	original := fileReadySleep
	fileReadySleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { fileReadySleep = original })
	err := c.WaitForUploadedFiles(context.Background(), a, []*UploadFileResult{{ID: "file-1", Status: "PENDING"}})
	var limited *FileRateLimitError
	if !errors.As(err, &limited) || queries != fileReadyPollAttempts {
		t.Fatalf("queries=%d err=%v", queries, err)
	}
}
