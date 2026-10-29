package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	powpkg "ds2api/pow"
)

type reuseTestUpstream struct {
	mu                       sync.Mutex
	uploads, fetches, proofs int
	status                   string
	fetchStatus              int
	missing                  bool
}

func (u *reuseTestUpstream) Do(req *http.Request) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	body, status := "", http.StatusOK
	switch req.URL.Path {
	case "/api/v0/chat/create_pow_challenge":
		u.proofs++
		hash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
		body = fmt.Sprintf(`{"code":0,"data":{"biz_code":0,"biz_data":{"challenge":{"algorithm":"DeepSeekHashV1","challenge":"%s","salt":"salt","expire_at":1712345678,"difficulty":1000,"signature":"sig"}}}}`, hex.EncodeToString(hash[:]))
	case "/api/v0/file/upload_file":
		u.uploads++
		body = fmt.Sprintf(`{"code":0,"data":{"biz_code":0,"biz_data":{"id":"file-%d","status":"%s","file_name":"photo.png","file_size":4,"is_image":true}}}`, u.uploads, u.status)
	case "/api/v0/file/fetch_files":
		u.fetches++
		body = fmt.Sprintf(`{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"%s","status":"%s","is_image":true}]}}}`, req.URL.Query().Get("file_ids"), u.status)
		if u.missing {
			body = `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[]}}}`
		}
		if u.fetchStatus != 0 {
			status = u.fetchStatus
		}
	case "/api/v0/chat_session/delete", "/api/v0/chat_session/delete_all":
		body = `{"code":0,"data":{"biz_code":0}}`
	default:
		return nil, fmt.Errorf("unexpected endpoint %s", req.URL.Path)
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func newReuseTestClient() (*Client, *reuseTestUpstream, *auth.RequestAuth, UploadFileRequest) {
	u := &reuseTestUpstream{status: "SUCCESS"}
	c := &Client{regular: u, maxRetries: 1, uploadCache: newUploadReuseCache(15*time.Minute, 64)}
	a := &auth.RequestAuth{CallerID: "caller", AccountID: "account", DeepSeekToken: "credential"}
	req := UploadFileRequest{Filename: "photo.png", ContentType: "image/png", Data: []byte("ABCD")}
	return c, u, a, req
}

func TestUploadReuseSkipsMultipartAndPowButValidatesFile(t *testing.T) {
	c, u, a, req := newReuseTestClient()
	first, err := c.UploadFile(context.Background(), a, req, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.UploadFilePending(context.Background(), a, req, 1)
	if err != nil || first.ID != second.ID || second.AccountID != a.AccountID {
		t.Fatalf("reuse failed: %#v %v", second, err)
	}
	if u.uploads != 1 || u.proofs != 1 || u.fetches != 1 {
		t.Fatalf("upload/proof/fetch counts: %d/%d/%d", u.uploads, u.proofs, u.fetches)
	}
	second.Status = "changed"
	third, err := c.UploadFile(context.Background(), a, req, 1)
	if err != nil || third.Status != "SUCCESS" {
		t.Fatalf("caller mutation leaked: %#v %v", third, err)
	}
}

func TestUploadReuseMissingAndRejectedFilesReupload(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			c, u, a, req := newReuseTestClient()
			first, err := c.UploadFilePending(context.Background(), a, req, 1)
			if err != nil {
				t.Fatal(err)
			}
			u.missing = missing
			if !missing {
				u.status = "FAILED"
			}
			second, err := c.UploadFilePending(context.Background(), a, req, 1)
			if err != nil || second.ID == first.ID || u.uploads != 2 {
				t.Fatalf("invalid file not reuploaded: %#v %v uploads=%d", second, err, u.uploads)
			}
		})
	}
}

func TestUploadReuseValidationErrorsDoNotReupload(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		c, u, a, req := newReuseTestClient()
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
			t.Fatal(err)
		}
		u.fetchStatus = status
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err == nil || u.uploads != 1 {
			t.Fatalf("status=%d err=%v uploads=%d", status, err, u.uploads)
		}
		u.fetchStatus = 0
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil || u.uploads != 1 {
			t.Fatalf("transient failure discarded reusable entry: %v", err)
		}
	}
}

func TestUploadReuseHTTPMissingFilesReupload(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		c, u, a, req := newReuseTestClient()
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
			t.Fatal(err)
		}
		u.fetchStatus = status
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil || u.uploads != 2 {
			t.Fatalf("status=%d uploads=%d err=%v", status, u.uploads, err)
		}
	}
}

func TestUploadReuseRefreshesOnlyOwningManagedAccount(t *testing.T) {
	t.Setenv("DS2API_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"owner@example.com","password":"test-password","token":"old-token"}]}`)
	store := config.LoadStore()
	acc, ok := store.FindAccount("owner@example.com")
	if !ok {
		t.Fatal("missing test account")
	}
	c, upstream, a, req := newReuseTestClient()
	a.UseConfigToken, a.Account, a.AccountID, a.DeepSeekToken = true, acc, acc.Identifier(), "old-token"
	logins := 0
	c.Auth = auth.NewResolver(store, nil, func(_ context.Context, account config.Account) (string, error) {
		logins++
		if account.Identifier() != a.AccountID {
			t.Fatal("refresh switched the file owner")
		}
		return "new-token", nil
	})
	c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/fetch_files") && r.Header.Get("Authorization") == "Bearer old-token" {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":401,"msg":"unauthorized"}`))}, nil
		}
		return upstream.Do(r)
	})
	first, err := c.UploadFilePending(context.Background(), a, req, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.UploadFilePending(context.Background(), a, req, 1)
	if err != nil || second.ID != first.ID || upstream.uploads != 1 || logins != 1 || a.DeepSeekToken != "new-token" {
		t.Fatalf("same-account refresh: file=%#v err=%v uploads=%d logins=%d", second, err, upstream.uploads, logins)
	}
}

func TestUploadReuseIsolationAndCompressionPolicy(t *testing.T) {
	for name, change := range map[string]func(*auth.RequestAuth, *UploadFileRequest){
		"caller":       func(a *auth.RequestAuth, _ *UploadFileRequest) { a.CallerID = "another" },
		"account":      func(a *auth.RequestAuth, _ *UploadFileRequest) { a.AccountID = "another" },
		"token":        func(a *auth.RequestAuth, _ *UploadFileRequest) { a.DeepSeekToken = "another" },
		"installation": func(a *auth.RequestAuth, _ *UploadFileRequest) { a.Account.InstallationID = "another" },
		"bytes":        func(_ *auth.RequestAuth, r *UploadFileRequest) { r.Data = []byte("EFGH") },
		"filename":     func(_ *auth.RequestAuth, r *UploadFileRequest) { r.Filename = "other.png" },
		"mime":         func(_ *auth.RequestAuth, r *UploadFileRequest) { r.ContentType = "application/octet-stream" },
		"purpose":      func(_ *auth.RequestAuth, r *UploadFileRequest) { r.Purpose = "another" },
		"thinking":     func(_ *auth.RequestAuth, r *UploadFileRequest) { r.ThinkingEnabled = true },
		"preserve":     func(_ *auth.RequestAuth, r *UploadFileRequest) { r.PreserveImage = true },
		"ratio":        func(_ *auth.RequestAuth, r *UploadFileRequest) { ratio := 0.5; r.PhotoCompressRatio = &ratio },
		"format":       func(_ *auth.RequestAuth, r *UploadFileRequest) { r.PictureCompressFormat = "jpg" },
	} {
		t.Run(name, func(t *testing.T) {
			c, u, a, req := newReuseTestClient()
			if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
				t.Fatal(err)
			}
			change(a, &req)
			if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil || u.uploads != 2 {
				t.Fatalf("scope/policy not isolated: uploads=%d err=%v", u.uploads, err)
			}
		})
	}
}

func TestUploadReuseInvalidatesAfterSessionDeletion(t *testing.T) {
	for _, all := range []bool{false, true} {
		c, u, a, req := newReuseTestClient()
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil {
			t.Fatal(err)
		}
		if all {
			if err := c.DeleteAllSessions(context.Background(), a); err != nil {
				t.Fatal(err)
			}
		} else if _, err := c.DeleteSession(context.Background(), a, "session", 1); err != nil {
			t.Fatal(err)
		}
		if _, err := c.UploadFilePending(context.Background(), a, req, 1); err != nil || u.uploads != 2 {
			t.Fatalf("delete did not invalidate: uploads=%d err=%v", u.uploads, err)
		}
	}
}
