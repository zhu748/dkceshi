package config

import "testing"

func TestAndroidInstallationMatchesAPKCipherVector(t *testing.T) {
	acc := Account{Email: "one@example.com", AndroidID: "0123456789abcdef", DeviceBrand: "vivo", DeviceID: "B-sdk-generated-id"}
	const want = "ialgvfoB0nGlpTPa7ow8KcV8Vpfmj8pqvMgCURmkxQA="
	if got := AndroidInstallationID(acc); got != want {
		t.Fatalf("APK AES vector: %q", got)
	}
	acc.DeviceID = "B-refreshed-sdk-id"
	if AndroidInstallationID(acc) != want {
		t.Fatal("SDK refresh changed installation ID")
	}
}

func TestAccountImageSettingsAreIsolatedFromConfigSnapshot(t *testing.T) {
	ratio := 0.8
	cfg := Config{Accounts: []Account{{Email: "one@example.com", PhotoCompressRatio: &ratio}}}
	copy := cfg.Clone()
	*copy.Accounts[0].PhotoCompressRatio = 0.2
	if *cfg.Accounts[0].PhotoCompressRatio != 0.8 {
		t.Fatal("snapshot changed stored image settings")
	}
}
