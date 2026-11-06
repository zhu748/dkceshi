package accounts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func accountTransferRequest(h *Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	RegisterRoutes(router, h)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(method, path, body))
	return rec
}

func TestAccountBackupRoundTripRestoresAllStoredFields(t *testing.T) {
	source := newAdminTestHandler(t, `{"keys":["SOURCE_KEY"],"accounts":[{
		"email":"state@example.com","mobile":"+14155552671","password":" p 中文 <&> ","token":"FULL_TOKEN",
		"name":"state","remark":"keep me","pool_type":"tools_only","priority":-7,"locale":"zh_CN",
		"disabled":true,"manual_disabled":true,"disabled_reason":"reason","banned":true,"auth_failed":true,
		"muted_until":2100000000.25,"cooldown_until":2100000010,"node_cooldown_until":2100000020,
		"no_proxy":true,"manual_binding":true,"device_model":"Pixel 6a","android_api_level":"35",
		"installation_id":"INSTALLATION","android_id":"ANDROID_ID","device_brand":"Google","timezone":"Asia/Shanghai",
		"rangers_id":"RANGERS","photo_compress_ratio":0.5,"picture_compress_format":"jpg",
		"hif_dliq":"DLIQ","hif_leim":"LEIM","hif_dliq_expires_at":9007199254740993,"hif_leim_expires_at":2100000000
	}]}`)
	if err := source.Store.UpdateAccountToken("state@example.com", "FULL_TOKEN"); err != nil {
		t.Fatal(err)
	}
	expected := source.Store.Accounts()[0]
	exported := accountTransferRequest(source, http.MethodGet, "/accounts/export?page_size=1&status=enabled", nil)
	if exported.Code != http.StatusOK || exported.Header().Get("Cache-Control") != "no-store" || !strings.Contains(exported.Header().Get("Content-Disposition"), ".json") {
		t.Fatalf("unexpected export response: %d %s", exported.Code, exported.Body.String())
	}
	if !strings.Contains(exported.Body.String(), "FULL_TOKEN") || !strings.Contains(exported.Body.String(), "9007199254740993") || strings.Contains(exported.Body.String(), "SOURCE_KEY") {
		t.Fatal("backup omitted credentials/precision or exported unrelated keys")
	}
	var backup accountBackup
	if err := json.Unmarshal(exported.Body.Bytes(), &backup); err != nil || len(backup.Accounts) != 1 || !reflect.DeepEqual(backup.Accounts[0], expected) {
		t.Fatalf("stored account was filtered by list parameters or fields lost: %v", err)
	}
	target := newAdminTestHandler(t, `{"keys":["TARGET_KEY"],"accounts":[{"email":"other@example.com","password":"untouched"}]}`)
	for attempt := 0; attempt < 2; attempt++ {
		rec := accountTransferRequest(target, http.MethodPost, "/accounts/import", exported.Body.Bytes())
		if rec.Code != http.StatusOK {
			t.Fatalf("import failed: %d %s", rec.Code, rec.Body.String())
		}
		actual, ok := target.Store.FindAccount(expected.Identifier())
		if !ok || !reflect.DeepEqual(actual, expected) || len(target.Store.Accounts()) != 2 {
			t.Fatalf("round trip changed account state or duplicated records: %+v", actual)
		}
		if !target.Store.HasAPIKey("TARGET_KEY") || target.Store.HasAPIKey("SOURCE_KEY") {
			t.Fatal("account import changed unrelated keys")
		}
	}
}

func TestAccountImportRestoresFalseAndZeroAndPreservesOtherAccounts(t *testing.T) {
	h := newAdminTestHandler(t, `{"accounts":[{"email":"a@example.com","password":"old","disabled":true,"manual_disabled":true,"banned":true,"auth_failed":true,"muted_until":2100000000,"cooldown_until":2100000000,"no_proxy":true},{"email":"other@example.com","password":"other"}]}`)
	rec := accountTransferRequest(h, http.MethodPost, "/accounts/import", []byte(`{"accounts":[{"email":"a@example.com","password":"new"}]}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	acc, _ := h.Store.FindAccount("a@example.com")
	if !acc.IsEnabled() || acc.Banned || acc.AuthFailed || acc.MutedUntil != 0 || acc.CooldownUntil != 0 || acc.NoProxy || acc.Password != "new" {
		t.Fatalf("omitted false/zero fields did not restore the exported defaults: %+v", acc)
	}
	if other, ok := h.Store.FindAccount("other@example.com"); !ok || other.Password != "other" {
		t.Fatal("unmentioned account changed")
	}
}

func TestInvalidAccountImportRejectsWholeBatch(t *testing.T) {
	for _, invalid := range []string{
		`null`, `[]`, `{"accounts":null}`, `{"format":"other","accounts":[{"email":"a@x.com"}]}`,
		`{"version":2,"accounts":[{"email":"a@x.com"}]}`,
		`[{"email":"new@x.com"},{"email":"bad@x.com","disabled":"false"}]`,
		`[{"email":"new@x.com"},{"email":"bad@x.com","muted_until":-1}]`,
		`[{"email":"new@x.com"},{"password":"missing identity"}]`,
		`[{"email":"new@x.com"},{"email":"bad@x.com","disabled":null}]`,
		`[{"email":"new@x.com"},{"email":"bad@x.com","unknown_switch":true}]`,
		`[{"email":"same@x.com"},{"email":"SAME@x.com"}]`,
		`[{"mobile":"13800138000"},{"mobile":"+8613800138000"}]`,
		`[{"email":"a@example.com","mobile":"+14155552672"}]`,
		`[{"email":"a@example.com"},{"mobile":"+14155552671"}]`,
		`[{"email":"new@x.com","proxy_id":"missing-proxy"}]`,
		`[{"email":"new@x.com","email":"repeated@x.com"}]`,
		`[{"email":"new@x.com","Email":"alias@x.com"}]`,
		`[{"email":"new@x.com"}] {}`, `[{"email":"new@x.com","pool_type":"wrong"}]`,
	} {
		h := newAdminTestHandler(t, `{"accounts":[{"email":"a@example.com","mobile":"+14155552671","password":"keep"},{"email":"b@example.com","mobile":"+14155552672","password":"also keep"}]}`)
		before := h.Store.Accounts()
		rec := accountTransferRequest(h, http.MethodPost, "/accounts/import", []byte(invalid))
		if rec.Code != http.StatusBadRequest || !reflect.DeepEqual(before, h.Store.Accounts()) {
			t.Fatalf("invalid batch partly applied: body=%s status=%d response=%s", invalid, rec.Code, rec.Body.String())
		}
	}
}

func TestAccountImportSizeBoundAndMobileMatching(t *testing.T) {
	h := newAdminTestHandler(t, `{"accounts":[{"mobile":"+8613800138000","password":"old"}]}`)
	rec := accountTransferRequest(h, http.MethodPost, "/accounts/import", bytes.Repeat([]byte(" "), maxAccountImportBytes+1))
	if rec.Code != http.StatusRequestEntityTooLarge || len(h.Store.Accounts()) != 1 {
		t.Fatal("oversized file was not bounded")
	}
	rec = accountTransferRequest(h, http.MethodPost, "/accounts/import", []byte(`[{"mobile":"13800138000","password":"new","disabled":true,"muted_until":2100000000}]`))
	if rec.Code != http.StatusOK || len(h.Store.Accounts()) != 1 {
		t.Fatalf("canonical mobile became a duplicate account: %s", rec.Body.String())
	}
	acc := h.Store.Accounts()[0]
	if acc.Mobile != "+8613800138000" || !acc.Disabled || acc.Password != "new" || acc.MutedUntil != 2100000000 {
		t.Fatal("mobile account state was lost")
	}
}

func TestAccountImportDoesNotRecalculateElasticSwitches(t *testing.T) {
	h := newAdminTestHandler(t, `{"elastic_pool":{"enabled":true,"global_count":1},"accounts":[]}`)
	rec := accountTransferRequest(h, http.MethodPost, "/accounts/import", []byte(`[{"email":"a@x.com","disabled":true,"manual_disabled":true},{"email":"b@x.com","disabled":true}]`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	accs := h.Store.Accounts()
	if !accs[0].Disabled || !accs[0].ManualDisabled || !accs[1].Disabled {
		t.Fatal("import immediately overwrote restored switches using elastic scheduling")
	}
}
