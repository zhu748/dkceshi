package accounts

import (
	"fmt"
	"net/http"
	"testing"
)

func TestQueueDeviceModeIsIndependentOfAndroidProfile(t *testing.T) {
	t.Setenv("VERCEL", "")
	t.Setenv("NOW_REGION", "")
	for _, mode := range []string{"manual", "auto"} {
		for _, supported := range []bool{false, true} {
			h := newAdminTestHandler(t, fmt.Sprintf(`{"runtime":{"device_id_mode":%q}}`, mode))
			if supported {
				h.DS = &fakeRealDeviceRefresher{}
			}
			rec := realDeviceRequest(t, h, http.MethodGet, "/admin/queue/status", "", h.queueStatus)
			body := decodeJSONBody(t, rec)
			if body["device_profile_kind"] != "android" || body["device_id_mode"] != mode || body["real_device_id_refresh_supported"] != (mode == "auto" && supported) {
				t.Fatalf("mode=%s supported=%v: %+v", mode, supported, body)
			}
		}
	}
}
