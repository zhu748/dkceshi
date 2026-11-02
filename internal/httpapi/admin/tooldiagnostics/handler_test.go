package tooldiagnostics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/tooldiagnostics"
	"github.com/go-chi/chi/v5"
)

func TestDiagnosticAPIRequiresExplicitFailureDeletionAndValidSettings(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	for i := 0; i < 3; i++ {
		record := tooldiagnostics.Record{Summary: tooldiagnostics.Summary{ID: fmt.Sprintf("tool_%d", i), CreatedAt: int64(i + 1), CompletedAt: 1, Pinned: i == 0, Status: "failed"}}
		if err := s.Put(record); err != nil {
			t.Fatal(err)
		}
	}
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: s})
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/tool-call-records?page=1&page_size=1", "", 200},
		{"GET", "/tool-call-records/tool_0", "", 200},
		{"GET", "/tool-call-records/missing", "", 404},
		{"PUT", "/tool-call-records/settings", `{"enabled":false,"limit":21}`, 400},
		{"DELETE", "/tool-call-records", "", 400},
		{"DELETE", "/tool-call-records?scope=normal", "", 200},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if got := s.List(tooldiagnostics.Query{}); got.Total != 1 || got.PinnedCount != 1 || !got.Settings.Enabled {
		t.Fatalf("API removed protected evidence: %+v", got)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/tool-call-records?scope=errors", nil))
	if w.Code != 200 || s.List(tooldiagnostics.Query{}).Total != 0 {
		t.Fatal("explicit failure deletion failed")
	}
}

func TestRepairSettingsAPIStoresKeyWithoutReturningIt(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: s})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/tool-call-records/repair-settings", strings.NewReader(`{"mode":"custom","base_url":"https://api.example/v1","model":"reasoner","api_key":"private-test-key","timeout_seconds":30}`)))
	if w.Code != 200 {
		t.Fatalf("save failed: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tool-call-records", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-test-key") || !strings.Contains(w.Body.String(), `"has_api_key":true`) {
		t.Fatalf("key leaked or settings missing: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/tool-call-records/settings", strings.NewReader(`{"enabled":false,"limit":50}`)))
	if w.Code != 200 || s.RepairSettings().Model != "reasoner" || s.RepairSettings().APIKey != "private-test-key" {
		t.Fatal("retention settings overwrote repair credentials")
	}
}

func TestModelDiscoveryBeforeSaveThenPersistsSelectedProtocolAndModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" || r.Header.Get("x-goog-api-key") != "draft-key" {
			t.Error("draft protocol or key not used")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": "models/chosen-model", "displayName": "Chosen model", "supportedGenerationMethods": []string{"generateContent"}}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "diag.json")
	s := tooldiagnostics.New(path)
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: s})
	draft := map[string]any{"mode": "custom", "protocol": "gemini", "base_url": server.URL, "api_key": "draft-key", "timeout_seconds": 30}
	body, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tool-call-records/repair-models", strings.NewReader(string(body))))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"chosen-model"`) || strings.Contains(w.Body.String(), "draft-key") {
		t.Fatalf("discovery failed: %d %s", w.Code, w.Body.String())
	}
	if s.RepairSettings().Mode != "original" {
		t.Fatal("discovery saved the draft")
	}
	draft["model"] = "chosen-model"
	body, err = json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/tool-call-records/repair-settings", strings.NewReader(string(body))))
	if w.Code != 200 {
		t.Fatalf("save selection failed: %s", w.Body.String())
	}
	got := tooldiagnostics.New(path).RepairSettings()
	if got.Protocol != "gemini" || got.Model != "chosen-model" || got.APIKey != "draft-key" {
		t.Fatal("selected model or protocol did not persist")
	}
}
