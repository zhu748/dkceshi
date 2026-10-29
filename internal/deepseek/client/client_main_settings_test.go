package client

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestCapturedUploadSettingsCacheAndImageOverrides(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_PHOTO_COMPRESS_RATIO", "")
	t.Setenv("DS2API_DEEPSEEK_PICTURE_COMPRESS_FORMAT", "")
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"biz_code": 0, "biz_data": map[string]any{"settings": fixture.Settings}}})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{mainSettings: &mainSettingsCache{entries: map[string]*mainSettingsEntry{}}}
	client.regular = alignmentDoer(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" || req.URL.RawQuery != "scope=main&did=did-test" || req.Header.Get("Content-Type") != "" {
			t.Fatalf("settings request: %s %s", req.Method, req.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(response))}, nil
	})
	a := &auth.RequestAuth{AccountID: "account", DeepSeekToken: "token", Account: config.Account{RangersID: "did-test"}}
	settings := client.uploadSettings(context.Background(), a)
	if settings.photoRatio == nil || *settings.photoRatio != 0.8 || settings.pictureFormat != "webp" || settings.filePollInterval != time.Second {
		t.Fatalf("settings=%+v", settings)
	}
	client.uploadSettings(context.Background(), a)
	if calls != 1 {
		t.Fatalf("settings fetched %d times", calls)
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, uploadTestImage()); err != nil {
		t.Fatal(err)
	}
	request := UploadFileRequest{Filename: "image.png", Data: pngData.Bytes()}
	prepared, err := prepareUploadImage(context.Background(), applyUploadSettings(request, a, settings))
	if err != nil || !strings.HasSuffix(prepared.Filename, ".webp") {
		t.Fatalf("remote compression not applied: %s %v", prepared.Filename, err)
	}
	request.PreserveImage = true
	preserved, err := prepareUploadImage(context.Background(), applyUploadSettings(request, a, settings))
	if err != nil || !bytes.Equal(preserved.Data, request.Data) {
		t.Fatal("preserve_image did not override remote compression")
	}
	explicit := 0.4
	a.Account.PhotoCompressRatio = &explicit
	if got := applyUploadSettings(request, a, settings).PhotoCompressRatio; got == nil || *got != explicit {
		t.Fatal("account override lost")
	}
	a.DeepSeekToken = "other-token"
	client.uploadSettings(context.Background(), a)
	if calls != 2 {
		t.Fatal("settings reused across credentials")
	}
}

func TestUploadSettingsFailurePreservesOriginalAndCoalescesRequests(t *testing.T) {
	client := &Client{mainSettings: &mainSettingsCache{entries: map[string]*mainSettingsEntry{}}}
	var calls atomic.Int32
	client.regular = alignmentDoer(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	a := &auth.RequestAuth{AccountID: "account", DeepSeekToken: "token"}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if got := client.uploadSettings(context.Background(), a); got.photoRatio != nil {
				t.Error("settings failure enabled compression")
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("parallel settings calls = %d", calls.Load())
	}
}
