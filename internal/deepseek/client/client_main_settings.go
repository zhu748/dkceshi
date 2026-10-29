package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

// Only the upload-related settings understood by this client are consumed.
// The APK default is no photo compression; a captured 0.8 is not a global default.
type mainSettings struct {
	photoRatio       *float64
	pictureFormat    string
	filePollInterval time.Duration
}

type mainSettingsEntry struct {
	settings  mainSettings
	expiresAt time.Time
	ready     chan struct{}
}

type mainSettingsCache struct {
	mu      sync.Mutex
	entries map[string]*mainSettingsEntry
}

func (c *Client) uploadSettings(ctx context.Context, a *auth.RequestAuth) mainSettings {
	if c.mainSettings == nil || a == nil || a.DeepSeekToken == "" {
		return mainSettings{}
	}
	headers := c.authHeadersForAuth(a)
	identity := a.AccountID + "\x00" + a.DeepSeekToken + "\x00" + headers["x-device-id"] + "\x00" + headers["x-rangers-id"] + "\x00" + headers["x-client-locale"]
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	cache := c.mainSettings
	cache.mu.Lock()
	if cache.entries == nil {
		cache.entries = make(map[string]*mainSettingsEntry)
	}
	if entry := cache.entries[key]; entry != nil {
		if entry.ready != nil {
			ready := entry.ready
			cache.mu.Unlock()
			select {
			case <-ready:
				return entry.settings
			case <-ctx.Done():
				return mainSettings{}
			}
		}
		if time.Now().Before(entry.expiresAt) {
			cache.mu.Unlock()
			return entry.settings
		}
	}
	entry := &mainSettingsEntry{ready: make(chan struct{})}
	if previous := cache.entries[key]; previous != nil {
		entry.settings = previous.settings
	}
	if len(cache.entries) >= 128 {
		for oldKey, old := range cache.entries {
			if oldKey != key && old.ready == nil && !time.Now().Before(old.expiresAt) {
				delete(cache.entries, oldKey)
			}
		}
	}
	cache.entries[key] = entry
	cache.mu.Unlock()

	settingsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	clients := c.requestClientsForAuth(settingsCtx, a)
	// Preserve the APK's scope, did query order, including an absent DID as empty.
	endpoint := dsprotocol.DeepSeekClientSettingsURL + "?scope=main&did=" + url.QueryEscape(headers["x-rangers-id"])
	response, status, err := c.getJSONWithStatus(auth.WithAuth(settingsCtx, a), clients.regular, endpoint, headers)
	code, bizCode, _, _ := extractResponseStatus(response)
	ttl := 30 * time.Second
	settings := entry.settings
	if err == nil && status == http.StatusOK && code == 0 && bizCode == 0 {
		data, _ := response["data"].(map[string]any)
		biz, _ := data["biz_data"].(map[string]any)
		values, _ := biz["settings"].(map[string]any)
		settings = parseUploadSettings(values, settings)
		ttl = 5 * time.Minute
	} else {
		config.Logger.Debug("[client_settings] using cached/default upload settings", "status", status, "error", err)
	}
	cache.mu.Lock()
	entry.settings, entry.expiresAt = settings, time.Now().Add(ttl)
	close(entry.ready)
	entry.ready = nil
	cache.mu.Unlock()
	return settings
}

func parseUploadSettings(values map[string]any, settings mainSettings) mainSettings {
	value := func(key string) (any, bool) {
		setting, ok := values[key].(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := setting["value"]
		return v, ok
	}
	if raw, ok := value("photo_picker_compress_ratio"); ok {
		settings.photoRatio = nil
		if ratio, valid := raw.(float64); valid && !math.IsNaN(ratio) && !math.IsInf(ratio, 0) && ratio >= 0 && ratio <= 1 {
			settings.photoRatio = &ratio
		}
	}
	if raw, ok := value("picture_compress_format"); ok {
		if format, valid := raw.(string); valid {
			settings.pictureFormat = "jpg"
			if format == "webp" {
				settings.pictureFormat = "webp"
			}
		}
	}
	if raw, ok := value("query_files_time_interval"); ok {
		if millis := toInt64(raw, 0); millis > 0 && millis <= 60_000 {
			settings.filePollInterval = time.Duration(millis) * time.Millisecond
		}
	}
	return settings
}

func applyUploadSettings(req UploadFileRequest, a *auth.RequestAuth, settings mainSettings) UploadFileRequest {
	if a != nil {
		if req.PhotoCompressRatio == nil {
			req.PhotoCompressRatio = a.Account.PhotoCompressRatio
		}
		if req.PictureCompressFormat == "" {
			req.PictureCompressFormat = a.Account.PictureCompressFormat
		}
	}
	// Explicit account/request/environment choices take precedence over remote defaults.
	if req.PhotoCompressRatio == nil && strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_PHOTO_COMPRESS_RATIO")) == "" {
		req.PhotoCompressRatio = settings.photoRatio
	}
	if req.PictureCompressFormat == "" && strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_PICTURE_COMPRESS_FORMAT")) == "" {
		req.PictureCompressFormat = settings.pictureFormat
	}
	return req
}
