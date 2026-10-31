package accounts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ds2api/internal/config"
	"ds2api/internal/devid"
	"github.com/go-chi/chi/v5"
)

func TestDeleteRemovesSDKArchive(t *testing.T) {
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	t.Setenv("DS2API_DEVICE_PROFILE_DIR", t.TempDir())
	h := newAdminTestHandler(t, `{"accounts":[{"email":"cleanup@example.com","password":"p"}]}`)
	p := devid.ProfilePathFor("cleanup@example.com")
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"version":3,"smid_v2":"old-device"}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Delete("/admin/accounts/{identifier}", h.deleteAccount)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/admin/accounts/cleanup@example.com", nil))
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	t.Logf("deleted path=%s; actual SDK path=%s", config.DeviceProfilePath("cleanup@example.com"), p)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("SDK archive survived account deletion")
	}
}

func TestRealDevicePaginationOverflow(t *testing.T) {
	h := newAdminTestHandler(t, `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"page@example.com","password":"p","device_id_type":"auto"}]}`)
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("large page panicked: %v", p)
		}
	}()
	h.listRealDeviceIDs(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/admin/device-ids/real?page=9223372036854775807&page_size=10", nil))
}
