package usage

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/usagestats"
)

func readUsage(h *Handler, condition string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/admin/usage", nil)
	r.Header.Set("If-None-Match", condition)
	w := httptest.NewRecorder()
	h.getUsage(w, r)
	return w
}

func TestUsageConditionalPolling(t *testing.T) {
	store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	h := &Handler{Usage: store}
	first := readUsage(h, "")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" {
		t.Fatalf("missing versioned response: %d %q", first.Code, tag)
	}
	for _, condition := range []string{tag, strings.TrimPrefix(tag, "W/"), `"other", ` + tag, "*"} {
		w := readUsage(h, condition)
		if w.Code != http.StatusNotModified || w.Body.Len() != 0 || w.Header().Get("ETag") != tag {
			t.Fatalf("condition %q: %d %s", condition, w.Code, w.Body.String())
		}
	}
	if got := readUsage(h, `W/"different"`); got.Code != http.StatusOK || got.Body.Len() == 0 {
		t.Fatal("mismatched version did not return a snapshot")
	}

	for _, mutate := range []func(){
		func() { store.Record("model", "caller", map[string]any{"prompt_tokens": 10}, time.Now()) },
		func() {
			if _, err := store.Import([]usagestats.Entry{{Date: "2026-11-03", Model: "model", TotalTokens: 20, Calls: 1}}); err != nil {
				t.Fatal(err)
			}
		},
		func() {
			if err := store.Clear(); err != nil {
				t.Fatal(err)
			}
		},
	} {
		mutate()
		next := readUsage(h, tag)
		if next.Code != http.StatusOK || next.Header().Get("ETag") == tag {
			t.Fatal("ledger mutation did not invalidate the validator")
		}
		tag = next.Header().Get("ETag")
	}
}

func TestUsageValidatorChangesAcrossStoreRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	first := usagestats.New(path)
	tag := readUsage(&Handler{Usage: first}, "").Header().Get("ETag")
	first.Close()
	second := usagestats.New(path)
	defer second.Close()
	if w := readUsage(&Handler{Usage: second}, tag); w.Code != http.StatusOK || w.Header().Get("ETag") == tag {
		t.Fatal("restarted store reused the previous validator")
	}
}

func BenchmarkUsagePolling(b *testing.B) {
	store := usagestats.New(filepath.Join(b.TempDir(), "usage.json"))
	defer store.Close()
	entries := make([]usagestats.Entry, 10000)
	for i := range entries {
		entries[i] = usagestats.Entry{Date: "2026-11-03", Model: "model", CallerID: fmt.Sprint(i), TotalTokens: 100, Calls: 1}
	}
	if _, err := store.Import(entries); err != nil {
		b.Fatal(err)
	}
	h := &Handler{Usage: store}
	tag := readUsage(h, "").Header().Get("ETag")
	for _, conditional := range []bool{false, true} {
		b.Run(fmt.Sprint(conditional), func(b *testing.B) {
			r := httptest.NewRequest(http.MethodGet, "/admin/usage", nil)
			if conditional {
				r.Header.Set("If-None-Match", tag)
			}
			var bytes int
			b.ReportAllocs()
			for b.Loop() {
				w := httptest.NewRecorder()
				h.getUsage(w, r)
				bytes = w.Body.Len()
			}
			b.ReportMetric(float64(bytes), "body-bytes/op")
		})
	}
}
