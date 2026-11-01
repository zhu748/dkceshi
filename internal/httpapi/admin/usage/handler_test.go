package usage

import (
	"ds2api/internal/usagestats"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportRejectsInvalidJSONAndCounters(t *testing.T) {
	for _, body := range []string{
		`null`,
		`{"entries":[{"date":"2026-11-01","model":"model","total_tokens":10,"calls":-1}]}`,
		`{"entries":[]} {"entries":[]}`,
	} {
		store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
		rec := httptest.NewRecorder()
		(&Handler{Usage: store}).importUsage(rec, httptest.NewRequest("POST", "/admin/usage/import", strings.NewReader(body)))
		if rec.Code != 400 || len(store.Summary()) != 0 {
			t.Errorf("invalid import must return 400 without mutating: %d %s", rec.Code, rec.Body.String())
		}
		store.Close()
	}
}
