package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountTransferRequiresAdmin(t *testing.T) {
	t.Setenv("DS2API_ADMIN_KEY", "admin")
	router := newHTTPAdminHarness(t, `{"accounts":[{"email":"private@example.com","password":"private-password"}]}`, nil)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/accounts/export"},
		{http.MethodPost, "/accounts/import"},
	} {
		for _, authorization := range []string{"", "Bearer incorrect"} {
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`[{"email":"private@example.com","password":"changed"}]`))
			req.Header.Set("Authorization", authorization)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "private-password") {
				t.Fatalf("unauthorized account transfer: status=%d body=%s", rec.Code, rec.Body.String())
			}
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts/export", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "private-password") || strings.Contains(rec.Body.String(), "changed") {
		t.Fatal("authorized export failed or rejected import changed credentials")
	}
}
