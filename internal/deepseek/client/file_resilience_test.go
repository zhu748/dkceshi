package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestFilePollingRefreshesOwnerAtMostOnce(t *testing.T) {
	for _, rejectedAgain := range []bool{false, true} {
		t.Run(fmt.Sprint(rejectedAgain), func(t *testing.T) {
			t.Setenv("DS2API_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
			t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"owner@example.com","password":"test-password","token":"old-token"}]}`)
			store := config.LoadStore()
			acc, ok := store.FindAccount("owner@example.com")
			if !ok {
				t.Fatal("missing owner account")
			}
			a := &auth.RequestAuth{UseConfigToken: true, AccountID: acc.Identifier(), Account: acc, DeepSeekToken: "old-token"}
			calls, logins := 0, 0
			c := &Client{regular: doerFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				status, body := 200, `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file","status":"SUCCESS"}]}}}`
				if rejectedAgain || r.Header.Get("Authorization") == "Bearer old-token" {
					status, body = 401, `{"code":401,"msg":"unauthorized"}`
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			c.Auth = auth.NewResolver(store, nil, func(_ context.Context, account config.Account) (string, error) {
				logins++
				if account.Identifier() != acc.Identifier() {
					t.Fatal("file owner changed")
				}
				return "new-token", nil
			})
			result := &UploadFileResult{ID: "file", AccountID: acc.Identifier(), Status: "PENDING"}
			err := c.WaitForUploadedFiles(context.Background(), a, []*UploadFileResult{result})
			if (err != nil) != rejectedAgain || calls != 2 || logins != 1 || a.AccountID != acc.Identifier() {
				t.Fatalf("refresh: calls=%d logins=%d account=%s err=%v", calls, logins, a.AccountID, err)
			}
		})
	}
}

func TestUploadReuseDoesNotTreatMalformedMetadataAsMissing(t *testing.T) {
	for _, body := range []string{"<html>gateway</html>", "", "null", "{}", `{"code":0,"data":{"biz_code":0}}`} {
		t.Run(body, func(t *testing.T) {
			c, upstream, a, req := newReuseTestClient()
			if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
				t.Fatal(err)
			}
			c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/fetch_files") {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				}
				return upstream.Do(r)
			})
			if _, err := c.UploadFilePending(context.Background(), a, req, 1); err == nil || errors.Is(err, ErrUploadFileNotFound) {
				t.Fatalf("malformed metadata must fail without marking the file missing: %v", err)
			}
			if upstream.uploads != 1 {
				t.Fatalf("malformed metadata caused %d uploads", upstream.uploads)
			}
		})
	}
}

func TestFilePollingStopsOnPermanentFailure(t *testing.T) {
	oldSleep := fileReadySleep
	fileReadySleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { fileReadySleep = oldSleep })
	for _, status := range []int{400, 401, 403, 422} {
		calls := 0
		c := &Client{regular: doerFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":1,"msg":"rejected"}`))}, nil
		})}
		err := c.WaitForUploadedFiles(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, []*UploadFileResult{{ID: "file", Status: "PENDING"}})
		if err == nil || calls != 1 {
			t.Errorf("status=%d: expected one request and an error, got calls=%d err=%v", status, calls, err)
		}
	}
}

func TestFilePollingRetriesTransientFailure(t *testing.T) {
	oldSleep := fileReadySleep
	fileReadySleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { fileReadySleep = oldSleep })
	for _, status := range []int{408, 429, 500, 503} {
		calls := 0
		c := &Client{regular: doerFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			responseStatus, body := status, `{"code":1}`
			if calls > 1 {
				responseStatus, body = 200, `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file","status":"SUCCESS"}]}}}`
			}
			return &http.Response{StatusCode: responseStatus, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		result := &UploadFileResult{ID: "file", Status: "PENDING"}
		err := c.WaitForUploadedFiles(context.Background(), &auth.RequestAuth{DeepSeekToken: "token"}, []*UploadFileResult{result})
		if err != nil || calls != 2 || result.Status != "SUCCESS" {
			t.Errorf("status=%d calls=%d file=%+v err=%v", status, calls, result, err)
		}
	}
}
