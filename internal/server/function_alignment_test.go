package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
)

func TestAlignedFeaturesAreWiredBehindAdminAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[],"keys":["test"]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "0")
	t.Setenv("DS2API_ADMIN_KEY", "local-test-admin")
	t.Setenv("DS2API_USAGE_PATH", filepath.Join(dir, "usage.json"))
	t.Setenv("DS2API_CHAT_HISTORY_PATH", filepath.Join(dir, "history.json"))
	t.Setenv("DS2API_ACCOUNT_DAILY_LIMIT_PATH", filepath.Join(dir, "daily.json"))
	t.Setenv("DS2API_STATIC_ADMIN_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>local-admin</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	token, err := auth.CreateJWTWithStore(1, app.Store)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/admin/usage", "/admin/usage/settings", "/admin/mihomo/status", "/admin/queue/status", "/admin/settings"} {
		for _, authorized := range []bool{false, true} {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Accept", "application/json")
			if authorized {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			recorder := httptest.NewRecorder()
			app.Router.ServeHTTP(recorder, request)
			want := http.StatusUnauthorized
			if authorized {
				want = http.StatusOK
			}
			if recorder.Code != want {
				t.Fatalf("%s authorized=%v: status=%d body=%s", path, authorized, recorder.Code, recorder.Body.String())
			}
			if authorized && path == "/admin/queue/status" {
				var body map[string]any
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body["device_profile_kind"] != "android" {
					t.Fatal("Android profile manager not advertised")
				}
			}
		}
	}
	for _, path := range []string{"/admin/settings", "/admin/test", "/admin/import", "/admin/usage"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "text/html")
		recorder := httptest.NewRecorder()
		app.Router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "local-admin") {
			t.Fatalf("navigation %s failed: status=%d", path, recorder.Code)
		}
	}
	for _, path := range []string{"/v1/audio/speech", "/audio/speech"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"input":"hello"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		app.Router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("speech route missing auth: %s -> %d", path, recorder.Code)
		}
	}
}
