package version

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func waitForRelease(t *testing.T, h *Handler) releaseStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.releases.mu.Lock()
		status := h.releases.status
		h.releases.mu.Unlock()
		if !status.pending {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background release check did not finish")
	return releaseStatus{}
}

func readVersion(t *testing.T, h *Handler) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	h.getVersion(rec, httptest.NewRequest("GET", "/admin/version", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || body["current_tag"] == "" || body["source"] == "" {
		t.Fatalf("missing local version: %s", rec.Body.String())
	}
	return body
}

func TestLocalVersionNeverWaitsForUpdateCheck(t *testing.T) {
	unblock := make(chan struct{})
	var calls atomic.Int32
	h := &Handler{checkRelease: func(ctx context.Context) (latestReleasePayload, error) {
		calls.Add(1)
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > releaseCheckTimeout {
			return latestReleasePayload{}, errors.New("unbounded check")
		}
		select {
		case <-unblock:
			return latestReleasePayload{TagName: "v99.0.0", HTMLURL: "https://example.com/release"}, nil
		case <-ctx.Done():
			return latestReleasePayload{}, ctx.Err()
		}
	}}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(unblock) }) })
	returned := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.getVersion(rec, httptest.NewRequest("GET", "/admin/version", nil))
		returned <- rec
	}()
	select {
	case rec := <-returned:
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["current_tag"] == nil {
			t.Fatal("missing immediate local version")
		}
		if body["update_check_pending"] != true {
			t.Fatalf("missing pending state: %#v", body)
		}
	case <-time.After(time.Second):
		t.Fatal("local version blocked on external network")
	}
	// Many tabs may open together; only one network request may run.
	for i := 0; i < 20; i++ {
		readVersion(t, h)
	}
	once.Do(func() { close(unblock) })
	waitForRelease(t, h)
	body := readVersion(t, h)
	if calls.Load() != 1 || body["latest_tag"] != "v99.0.0" || body["update_check_pending"] != false {
		t.Fatalf("cache not reused: calls=%d body=%#v", calls.Load(), body)
	}
}

func TestFailedUpdateChecksBackOffAndRetainLastRelease(t *testing.T) {
	var calls atomic.Int32
	h := &Handler{checkRelease: func(context.Context) (latestReleasePayload, error) {
		if calls.Add(1) == 1 {
			return latestReleasePayload{TagName: "v99.0.0"}, nil
		}
		return latestReleasePayload{}, errors.New("offline")
	}}
	readVersion(t, h)
	waitForRelease(t, h)
	h.releases.mu.Lock()
	h.releases.nextCheck = time.Time{}
	h.releases.mu.Unlock()
	body := readVersion(t, h)
	if body["latest_tag"] != "v99.0.0" {
		t.Fatal("stale release disappeared while refreshing")
	}
	status := waitForRelease(t, h)
	if status.err != "offline" {
		t.Fatalf("failure was lost: %#v", status)
	}
	for i := 0; i < 10; i++ {
		body = readVersion(t, h)
	}
	if calls.Load() != 2 || body["latest_tag"] != "v99.0.0" || body["check_error"] != "offline" {
		t.Fatalf("failed checks not cached: calls=%d body=%#v", calls.Load(), body)
	}
}
