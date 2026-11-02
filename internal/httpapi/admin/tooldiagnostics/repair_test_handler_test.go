package tooldiagnostics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/tooldiagnostics"
	"github.com/go-chi/chi/v5"
)

func TestRepairTestDraftDoesNotSaveAndErrorsDoNotExposeCredential(t *testing.T) {
	const key = "saved-probe-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("saved credential not resolved")
		}
		w.WriteHeader(401)
		if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "invalid " + key}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	if err := s.SetRepairSettings(tooldiagnostics.RepairSettings{Mode: "custom", BaseURL: server.URL, Model: "saved-model", APIKey: key, TimeoutSeconds: 5}); err != nil {
		t.Fatal(err)
	}
	_, revision := s.SettingsSnapshot()
	router := chi.NewRouter()
	RegisterRoutes(router, &Handler{Store: s})
	body, err := json.Marshal(tooldiagnostics.RepairSettings{Protocol: "openai", BaseURL: server.URL, Model: "unsaved-model", TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/tool-call-records/repair-test", strings.NewReader(string(body))))
	if w.Code != 502 || strings.Contains(w.Body.String(), key) || !strings.Contains(w.Body.String(), "[redacted]") {
		t.Fatalf("bad probe error: %d %s", w.Code, w.Body.String())
	}
	_, after := s.SettingsSnapshot()
	if after != revision || s.RepairSettings().Model != "saved-model" || s.List(tooldiagnostics.Query{}).Total != 0 {
		t.Fatal("probe mutated production settings or diagnostics")
	}
}
