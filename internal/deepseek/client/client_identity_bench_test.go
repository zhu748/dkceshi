package client

import (
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

// BaseHeadersForAuth runs 5-8 times per completion chain (session, PoW,
// completion, continue, prefetch). The benchmark pins a fully-populated
// account profile, the steady state for managed accounts after
// EnsureAccountDeviceProfile has persisted the device fields once.
func BenchmarkBaseHeadersForAuthCompleteProfile(b *testing.B) {
	acc := config.Account{
		Email:           "acc@example.com",
		Token:           "token",
		DeviceModel:     "Pixel 9a",
		AndroidAPILevel: "36",
		AndroidID:       "0123456789abcdef",
		InstallationID:  "install-id-0123456789",
		DeviceBrand:     "google",
		Timezone:        "Asia/Shanghai",
		Locale:          "zh-CN",
	}
	a := &auth.RequestAuth{Account: acc, AccountID: acc.Identifier(), UseConfigToken: true, DeepSeekToken: "token"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		headers := BaseHeadersForAuth(a)
		if headers["x-device-model"] == "" {
			b.Fatalf("missing device model header")
		}
	}
}
