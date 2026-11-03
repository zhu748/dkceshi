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

	"ds2api/internal/auth"
)

func TestUploadBusinessRateLimitBacksOffAndPreservesOwner(t *testing.T) {
	for _, envelope := range []string{
		`{"code":40029,"msg":"TOO_MANY_REQUESTS"}`,
		`{"code":0,"data":{"biz_code":40029,"biz_msg":"TOO_MANY_REQUESTS"}}`,
	} {
		for _, recover := range []bool{true, false} {
			c, upstream, a, req := newReuseTestClient()
			attempts := 0
			c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/upload_file") {
					attempts++
					if !recover || attempts < 3 {
						return &http.Response{StatusCode: 200, Header: http.Header{"Retry-After": []string{"3"}}, Body: io.NopCloser(strings.NewReader(envelope))}, nil
					}
				}
				return upstream.Do(r)
			})
			var delays []time.Duration
			original := fileReadySleep
			fileReadySleep = func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }
			file, err := c.UploadFilePending(context.Background(), a, req, 3)
			fileReadySleep = original
			if attempts != 3 || !reflect.DeepEqual(delays, []time.Duration{3 * time.Second, 3 * time.Second}) || a.AccountID != "account" {
				t.Fatalf("attempts=%d delays=%v owner=%s", attempts, delays, a.AccountID)
			}
			if recover {
				if err != nil || file == nil || file.ID == "" {
					t.Fatalf("upload did not recover: %v", err)
				}
			} else {
				var limited *FileRateLimitError
				if !errors.As(err, &limited) || limited.HTTPStatus() != 429 {
					t.Fatalf("lost rate limit: %v", err)
				}
			}
		}
	}
}

func TestUploadRateLimitCancellationDoesNotResend(t *testing.T) {
	c, upstream, a, req := newReuseTestClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/upload_file") {
			attempts++
			cancel()
			return &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":40029}`))}, nil
		}
		return upstream.Do(r)
	})
	_, err := c.UploadFilePending(ctx, a, req, 3)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestFilePollingRecoversFromHTTP200RateLimit(t *testing.T) {
	oldSleep := fileReadySleep
	var delays []time.Duration
	fileReadySleep = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	t.Cleanup(func() { fileReadySleep = oldSleep })
	calls := 0
	c := &Client{regular: doerFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body := `{"code":40029,"msg":"TOO_MANY_REQUESTS"}`
		if calls == 3 {
			body = `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file","status":"SUCCESS"}]}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	file := &UploadFileResult{ID: "file", Status: "PENDING"}
	err := c.WaitForUploadedFiles(context.Background(), &auth.RequestAuth{}, []*UploadFileResult{file})
	if err != nil || calls != 3 || file.Status != "SUCCESS" || !reflect.DeepEqual(delays, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatalf("calls=%d delays=%v file=%+v err=%v", calls, delays, file, err)
	}
}
