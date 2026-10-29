package protocol

import (
	"encoding/json"
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
	// 真实 Android App 不发送 Accept-Language 头，项目应对齐
	if _, ok := BaseHeaders["Accept-Language"]; ok {
		t.Fatalf("unexpected Accept-Language header present, value=%q", BaseHeaders["Accept-Language"])
	}
	if BaseHeaders["x-client-timezone-offset"] != "28800" {
		t.Fatalf("unexpected base header x-client-timezone-offset=%q", BaseHeaders["x-client-timezone-offset"])
	}
	if BaseHeaders["x-rangers-id"] != "" {
		t.Fatalf("unregistered telemetry ID must not be fabricated")
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

func TestBaseHeadersDoNotInventTelemetryRegistration(t *testing.T) {
	prev := explicitRangersID
	explicitRangersID = ""
	defer func() { explicitRangersID = prev }()

	base := BaseHeadersForRangersSeed("account-a")
	seeded := BaseHeadersForRangersSeed("account-b")
	if base["x-rangers-id"] != "" || seeded["x-rangers-id"] != "" {
		t.Fatal("account seed must not be used as a telemetry DID")
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

func TestSharedConstantsHaveNoFixedDeviceModel(t *testing.T) {
	var shared sharedConstants
	if err := json.Unmarshal(sharedConstantsJSON, &shared); err != nil {
		t.Fatal(err)
	}
	if shared.Client.DeviceModel != "" {
		t.Fatalf("unexpected fixed device default: %s", shared.Client.DeviceModel)
	}
	headers := buildBaseHeaders(normalizeClientConstants(shared.Client), shared.BaseHeaders)
	if headers["x-device-model"] != "" {
		t.Fatal("missing device profile must not silently become a captured model")
	}
}
