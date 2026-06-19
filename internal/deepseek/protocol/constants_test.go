package protocol

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedConstantsLoaded(t *testing.T) {
	cfg := sharedConstants{}
	if err := json.Unmarshal(sharedConstantsJSON, &cfg); err != nil {
		t.Fatalf("failed to parse shared constants: %v", err)
	}
	client := normalizeClientConstants(cfg.Client)
	if ClientVersion != client.Version {
		t.Fatalf("unexpected client version=%q", ClientVersion)
	}
	wantUserAgent := client.Name + "/" + client.Version + " Android/" + client.AndroidAPILevel
	if BaseHeaders["User-Agent"] != wantUserAgent {
		t.Fatalf("unexpected user agent=%q", BaseHeaders["User-Agent"])
	}
	if BaseHeaders["x-client-platform"] != "android" {
		t.Fatalf("unexpected base header x-client-platform=%q", BaseHeaders["x-client-platform"])
	}
	if BaseHeaders["x-client-version"] != ClientVersion {
		t.Fatalf("unexpected base header x-client-version=%q", BaseHeaders["x-client-version"])
	}
	if BaseHeaders["Content-Type"] != "application/json" {
		t.Fatalf("unexpected base header Content-Type=%q", BaseHeaders["Content-Type"])
	}
	if BaseHeaders["x-client-bundle-id"] != "com.deepseek.chat" {
		t.Fatalf("unexpected base header x-client-bundle-id=%q", BaseHeaders["x-client-bundle-id"])
	}
	if BaseHeaders["x-client-locale"] != "zh_CN" {
		t.Fatalf("unexpected base header x-client-locale=%q", BaseHeaders["x-client-locale"])
	}
	if BaseHeaders["Accept-Language"] != "zh-CN,zh;q=0.9" {
		t.Fatalf("unexpected base header Accept-Language=%q", BaseHeaders["Accept-Language"])
	}
	if BaseHeaders["x-client-timezone-offset"] != "28800" {
		t.Fatalf("unexpected base header x-client-timezone-offset=%q", BaseHeaders["x-client-timezone-offset"])
	}
	if BaseHeaders["x-rangers-id"] == "" {
		t.Fatalf("expected x-rangers-id to be derived from seed")
	}
	if len(SkipContainsPatterns) == 0 {
		t.Fatal("expected skip contains patterns to be loaded")
	}
	if _, ok := SkipExactPathSet["response/search_status"]; !ok {
		t.Fatal("expected response/search_status in exact skip path set")
	}
}

func TestClientHeadersDerivedFromSharedVersion(t *testing.T) {
	client := normalizeClientConstants(clientConstants{
		Name:            "DeepSeek",
		Platform:        "android",
		Version:         "9.8.7",
		AndroidAPILevel: "35",
		Locale:          "zh_CN",
	})
	headers := buildBaseHeaders(client, map[string]string{
		"User-Agent":       "stale",
		"x-client-version": "stale",
	})
	if headers["User-Agent"] != "DeepSeek/9.8.7 Android/35" {
		t.Fatalf("unexpected derived user agent=%q", headers["User-Agent"])
	}
	if headers["x-client-version"] != "9.8.7" {
		t.Fatalf("unexpected derived client version=%q", headers["x-client-version"])
	}
}

func TestAcceptLanguageFromLocale(t *testing.T) {
	cases := []struct {
		locale string
		want   string
	}{
		{locale: "zh_CN", want: "zh-CN,zh;q=0.9"},
		{locale: "en_US", want: "en-US,en;q=0.9"},
		{locale: "ja", want: "ja"},
		{locale: "", want: ""},
	}
	for _, tc := range cases {
		if got := acceptLanguageFromLocale(tc.locale); got != tc.want {
			t.Fatalf("locale=%q got=%q want=%q", tc.locale, got, tc.want)
		}
	}
}

func TestBaseHeadersForRangersSeedUsesAccountSeed(t *testing.T) {
	prev := explicitRangersID
	explicitRangersID = ""
	defer func() { explicitRangersID = prev }()

	base := BaseHeadersForRangersSeed("account-a")
	seeded := BaseHeadersForRangersSeed("account-b")
	if base["x-rangers-id"] == seeded["x-rangers-id"] {
		t.Fatalf("expected per-seed rangers id to differ, got %q for both", base["x-rangers-id"])
	}
}

func TestExplicitRangersEnvOverrides(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_RANGERS_ID", "7890123456789012345")
	prev := explicitRangersID
	explicitRangersID = ""
	defer func() { explicitRangersID = prev }()

	applyEnvClientOverrides(clientConstants{})
	if explicitRangersID == "" {
		t.Fatal("expected explicit rangers id to be populated from env")
	}
}

func TestDeriveRangersIDStablePerSeed(t *testing.T) {
	a := deriveRangersIDFromSeed("seed-x")
	b := deriveRangersIDFromSeed("seed-x")
	if a != b {
		t.Fatalf("expected deterministic seed-derived id, got %q vs %q", a, b)
	}
	if a == "" {
		t.Fatal("expected non-empty derived id")
	}
}

func TestDeriveRangersIDDiffersAcrossSeeds(t *testing.T) {
	a := deriveRangersIDFromSeed("alpha")
	b := deriveRangersIDFromSeed("beta")
	if a == b {
		t.Fatalf("expected different ids across seeds, got %q for both", a)
	}
}

func TestDeriveRangersIDHonorsHostEnv(t *testing.T) {
	prev, hadPrev := os.LookupEnv("DS2API_DEEPSEEK_RANGERS_SEED")
	defer func() {
		if hadPrev {
			_ = os.Setenv("DS2API_DEEPSEEK_RANGERS_SEED", prev)
		} else {
			_ = os.Unsetenv("DS2API_DEEPSEEK_RANGERS_SEED")
		}
	}()
	_ = os.Unsetenv("DS2API_DEEPSEEK_RANGERS_SEED")
	base := deriveDefaultRangersID()

	_ = os.Setenv("DS2API_DEEPSEEK_RANGERS_SEED", "stable-seed")
	withSeed := deriveDefaultRangersID()
	if base == withSeed {
		t.Fatalf("expected default rangers id to vary with DS2API_DEEPSEEK_RANGERS_SEED, got %q for both", base)
	}
	if _, err := os.Stat("/dev/null"); err == nil {
		// Sanity: derivation function on the seed alone is deterministic.
		if a := deriveRangersIDFromSeed("stable-seed"); a != deriveRangersIDFromSeed("stable-seed") {
			t.Fatalf("deriveRangersIDFromSeed not deterministic: %q vs %q", a, deriveRangersIDFromSeed("stable-seed"))
		}
	}
}
