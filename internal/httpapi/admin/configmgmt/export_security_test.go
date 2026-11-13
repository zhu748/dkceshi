package configmgmt

import (
	"ds2api/internal/config"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegressionHTTPExportMustOmitInternalSecretAndRuntimeToken(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"tool_marker_secret":"review-internal-secret","accounts":[{"email":"a@example.com","password":"pwd"}]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "")
	s, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateAccountToken("a@example.com", "review-runtime-token"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	(&Handler{Store: s}).configExport(rec, httptest.NewRequest("GET", "/admin/config/export", nil))
	for _, secret := range []string{"review-internal-secret", "review-runtime-token"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("HTTP export leaks %s in config field", secret)
		}
	}
}
