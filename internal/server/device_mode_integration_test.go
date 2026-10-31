package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Exercise the real router from settings through login without an upstream
// account: missing local SDK assets must fail before any login request is sent.
func TestDeviceModeSettingsAndSDKLoginFailureThroughRouter(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for SDK subprocess integration")
	}
	version, err := exec.Command(node, "-p", "process.versions.node.split('.')[0]").Output()
	if err != nil {
		t.Skip("Node.js version could not be checked")
	}
	major, err := strconv.Atoi(strings.TrimSpace(string(version)))
	if err != nil || major < 22 {
		t.Skip("Node.js 22+ is required by the SDK runner")
	}
	script, err := filepath.Abs("../../scripts/devid-generate.cjs")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	t.Setenv("DS2API_ADMIN_KEY", "local-router-device-test")
	t.Setenv("DS2API_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("DS2API_CHAT_HISTORY_PATH", filepath.Join(t.TempDir(), "history.json"))
	t.Setenv("DS2API_USAGE_PATH", filepath.Join(t.TempDir(), "usage.json"))
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	t.Setenv("DS2API_DEVICE_PROFILE_DIR", t.TempDir())
	t.Setenv("DS2API_DEVID_SCRIPT", script)
	t.Setenv("DS2API_CONFIG_JSON", `{"runtime":{"device_id_mode":"manual"},"accounts":[{"email":"fixture@example.invalid","password":"fixture"}],"keep_alive":{"enabled":false},"mihomo":{"enabled":false}}`)
	app, err := NewApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	call := func(method, path, body, token string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		app.Router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s: status=%d body=%s", method, path, rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	login := call(http.MethodPost, "/admin/login", `{"admin_key":"local-router-device-test"}`, "")
	token, ok := login["token"].(string)
	if !ok || token == "" {
		t.Fatal("missing admin token")
	}
	before := call(http.MethodGet, "/admin/settings", "", token)
	if before["runtime"].(map[string]any)["device_id_mode"] != "manual" {
		t.Fatal("unexpected initial mode")
	}
	if call(http.MethodPut, "/admin/settings", `{"runtime":{"device_id_mode":"auto"}}`, token)["success"] != true {
		t.Fatal("settings save failed")
	}
	after := call(http.MethodGet, "/admin/settings", "", token)
	if after["runtime"].(map[string]any)["device_id_mode"] != "auto" {
		t.Fatal("saved mode did not become effective")
	}
	queue := call(http.MethodGet, "/admin/queue/status", "", token)
	if queue["device_profile_kind"] != "android" || queue["device_id_mode"] != "auto" || queue["real_device_id_refresh_supported"] != false {
		t.Fatalf("inconsistent queue capability: %+v", queue)
	}
	sdk, _ := queue["device_id_sdk"].(map[string]any)
	if sdk["ready"] != false || !strings.Contains(fmt.Sprint(sdk["error"]), "SDK 资源不完整") {
		t.Fatalf("missing SDK assets were not diagnosed: %+v", sdk)
	}
	if call(http.MethodGet, "/admin/device-ids/real", "", token)["mode"] != "auto" {
		t.Fatal("SDK ID manager unavailable")
	}
	result := call(http.MethodPost, "/admin/accounts/test", `{"identifier":"fixture@example.invalid"}`, token)
	message, _ := result["message"].(string)
	if result["success"] != false || !strings.Contains(message, "SDK 资源不完整") || !strings.Contains(message, "fp-1.min.js") {
		t.Fatalf("SDK stdout error was not preserved by login API: %s", message)
	}
}
