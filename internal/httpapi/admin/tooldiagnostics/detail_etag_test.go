package tooldiagnostics

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ds2api/internal/tooldiagnostics"
	"github.com/go-chi/chi/v5"
)

func TestDetailETagTracksCommittedVersion(t *testing.T) {
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	record := tooldiagnostics.Record{Summary: tooldiagnostics.Summary{ID: "tool_etag", Revision: 1, CompletedAt: 1}}
	if err := store.Put(record); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: store})
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest("GET", "/tool-call-records/tool_etag", nil))
	etag := first.Header().Get("ETag")
	if first.Code != 200 || etag == "" {
		t.Fatal(first.Code, etag)
	}
	req := httptest.NewRequest("GET", "/tool-call-records/tool_etag", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	router.ServeHTTP(second, req)
	if second.Code != 304 || second.Body.Len() != 0 {
		t.Fatal(second.Code, second.Body.String())
	}
	// Even a retry with the same recorder revision has a new committed version.
	record.Error = "new evidence"
	if err := store.Put(record); err != nil {
		t.Fatal(err)
	}
	third := httptest.NewRecorder()
	router.ServeHTTP(third, req)
	if third.Code != 200 || third.Header().Get("ETag") == etag {
		t.Fatal("new detail reused old ETag")
	}
}

func TestUnchangedETagDoesNotReadDetailFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag.json")
	store := tooldiagnostics.New(path)
	const id = "tool_cached"
	if err := store.Put(tooldiagnostics.Record{Summary: tooldiagnostics.Summary{ID: id, Revision: 1, CompletedAt: 1}}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	tag := diagnosticETag(record.Summary)
	if err := os.Remove(filepath.Join(path+".d", id+"."+record.DetailVersion+".json")); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: store})
	for _, condition := range []string{tag, "W/" + tag, `"another-version", W/` + tag, "*"} {
		req := httptest.NewRequest("GET", "/tool-call-records/"+id, nil)
		req.Header.Set("If-None-Match", condition)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != 304 || response.Body.Len() != 0 {
			t.Fatalf("conditional request tried to read detail: %s => %d %s", condition, response.Code, response.Body.String())
		}
	}
}
