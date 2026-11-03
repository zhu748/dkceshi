package usage

import (
	"ds2api/internal/usagestats"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
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

func TestSettingsRejectInvalidPeakWindowsWithoutChangingSavedValues(t *testing.T) {
	for _, peak := range []usagestats.PeakSettings{
		{Enabled: true, Start1: "09:00", End1: "12:00", Start2: "11:00", End2: "14:00"},
		{Enabled: true, Start1: "09:00suffix", End1: "12:00"},
	} {
		store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
		defer store.Close()
		before := store.Settings()
		settings := store.Settings()
		settings.Peak = peak
		body, err := json.Marshal(map[string]any{"settings": settings})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		(&Handler{Usage: store}).updateUsageSettings(rec, httptest.NewRequest("PUT", "/admin/usage/settings", strings.NewReader(string(body))))
		if rec.Code != 400 || !reflect.DeepEqual(before, store.Settings()) {
			t.Fatalf("invalid settings must return 400 and retain values: %d %s", rec.Code, rec.Body.String())
		}
	}
}
