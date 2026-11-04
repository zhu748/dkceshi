package responses

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cleanupRecordingDS struct {
	responsesHistoryDS
	deleted atomic.Int32
	done    chan struct{}
}

func (d *cleanupRecordingDS) DeleteSession(context.Context, *auth.RequestAuth, string, int) (*dsclient.DeleteSessionResult, error) {
	d.deleted.Add(1)
	d.done <- struct{}{}
	return &dsclient.DeleteSessionResult{Success: true}, nil
}
func (d *cleanupRecordingDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	d.deleted.Add(1)
	d.done <- struct{}{}
	return &dsclient.DeleteSessionResult{Success: true}, nil
}
func TestResponsesHonorsAutomaticSessionCleanup(t *testing.T) {
	store, resolver := newDirectTokenResolver(t)
	if err := store.Update(func(c *config.Config) error { c.AutoDelete.Mode = "single"; return nil }); err != nil {
		t.Fatal(err)
	}
	ds := &cleanupRecordingDS{done: make(chan struct{}, 2)}
	h := &Handler{Store: store, Auth: resolver, DS: ds}
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"deepseek-v4.1-flash","input":"hello"}`))
	req.Header.Set("Authorization", "Bearer direct-token")
	rec := httptest.NewRecorder()
	h.Responses(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	select {
	case <-ds.done:
	case <-time.After(time.Second):
		t.Fatal("session cleanup timed out")
	}
	if ds.deleted.Load() == 0 {
		t.Fatal("successful Responses request never deletes its upstream session with auto_delete.mode=single")
	}
}
