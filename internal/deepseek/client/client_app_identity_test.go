package client

import (
	"context"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/devid"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAutoLoginGeneratesRiskIDAndKeepsInstallationForDeviceCheck(t *testing.T) {
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	t.Setenv("DS2API_CONFIG_JSON", `{"runtime":{"device_id_mode":"auto"},"accounts":[{"email":"auto@example.com","password":"p","device_model":"V2507A","android_api_level":"36"}]}`)
	store := config.LoadStore()
	c := NewClient(store, nil)
	defer c.Close()
	generated := 0
	c.deviceIDGenerator = func(_ context.Context, opts devid.Options) (string, error) {
		generated++
		if _, err := os.Stat(opts.AnchorPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(opts.ProfilePath, []byte(`{"version":3,"profile":{}}`), 0o600); err != nil {
			return "", err
		}
		return "B-generated-risk-id", nil
	}
	var installation string
	seen := []string{}
	c.regular = alignmentDoer(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		headerID := r.Header.Get("x-device-id")
		if headerID == "" || headerID == "B-generated-risk-id" {
			t.Fatal("risk ID leaked to installation header")
		}
		if installation == "" {
			installation = headerID
		}
		if installation != headerID {
			t.Fatal("installation changed")
		}
		seen = append(seen, r.URL.Path)
		raw := `{"code":0,"data":{"biz_code":0,"biz_data":{"user":{"token":"login-token"}}}}`
		if strings.HasSuffix(r.URL.Path, "/login") {
			if body["device_id"] != "B-generated-risk-id" {
				t.Fatal("login lost SDK result")
			}
		} else {
			if body["device_id"] != installation {
				t.Fatal("device check lost installation identity")
			}
			raw = `{"code":0,"data":{"biz_code":0,"biz_data":{"rotate":{"token":"rotated-token"}}}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	for range 2 {
		if token, err := c.Login(context.Background(), config.Account{Email: "auto@example.com", Password: "p"}); err != nil || token != "rotated-token" {
			t.Fatalf("login %s %v", token, err)
		}
	}
	if generated != 1 || len(seen) != 4 {
		t.Fatalf("generation=%d requests=%v", generated, seen)
	}
	stored, _ := store.FindAccount("auto@example.com")
	if stored.DeviceIDType != config.DeviceIDTypeAuto || stored.DeviceID != "B-generated-risk-id" || stored.InstallationID != installation {
		t.Fatal("identities not persisted")
	}
}
func TestTimezoneAnchorMatchesAccountHeaderAcrossDST(t *testing.T) {
	t.Setenv("DS2API_DEVID_DIR", t.TempDir())
	acc := config.Account{Email: "zone@example.com", DeviceModel: "Pixel 9", AndroidAPILevel: "36", Timezone: "America/Los_Angeles"}
	path, err := buildDevidAnchor(acc.Identifier(), acc)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var anchor map[string]any
	if err := json.Unmarshal(raw, &anchor); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation(acc.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	_, offset := time.Now().In(loc).Zone()
	if anchor["tz"] != acc.Timezone || anchor["tz_offset"] != float64(-offset/60) {
		t.Fatalf("anchor=%v", anchor)
	}
	if got := BaseHeadersForAuth(&auth.RequestAuth{Account: acc})["x-client-timezone-offset"]; got != strconv.Itoa(offset) {
		t.Fatalf("header offset=%s", got)
	}
}

func TestAttestationHeadersExpireIndependently(t *testing.T) {
	acc := config.Account{Email: "hif@example.com", HIFDliq: "real-dliq", HIFDliqExpiresAt: time.Now().Unix() + 60, HIFLeim: "expired-leim", HIFLeimExpiresAt: time.Now().Unix() - 1}
	headers := BaseHeadersForAuth(&auth.RequestAuth{Account: acc})
	if headers["x-hif-dliq"] != "real-dliq" || headers["x-hif-leim"] != "" || headers["x-rangers-id"] != "" {
		t.Fatalf("headers=%v", headers)
	}
}
