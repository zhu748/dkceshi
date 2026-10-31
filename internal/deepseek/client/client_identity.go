package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

// androidAPILevelPattern is package-level because BaseHeadersForAuth runs on
// every upstream request; compiling it per call was pure overhead.
var androidAPILevelPattern = regexp.MustCompile(`Android/[0-9]+`)

// locationCache memoizes time.LoadLocation results per IANA name. The Go
// stdlib re-reads and re-parses the zoneinfo file on every call, and
// BaseHeadersForAuth (5-8 calls per completion chain) used to hit the disk
// each time an account had a timezone configured. *time.Location values are
// immutable, so sharing them is safe.
var locationCache sync.Map // string -> *time.Location

func loadCachedLocation(name string) (*time.Location, error) {
	if v, ok := locationCache.Load(name); ok {
		if loc, ok := v.(*time.Location); ok {
			return loc, nil
		}
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	locationCache.Store(name, loc)
	return loc, nil
}

// BaseHeadersForAuth is shared with Vercel prepare so upload, completion and
// continuation requests retain one account's configured installation identity.
// Captured identity values are deliberately not embedded in the program.
func BaseHeadersForAuth(a *auth.RequestAuth) map[string]string {
	seed := accountHeaderSeed(a)
	if a != nil && seed == "" {
		seed = strings.TrimSpace(a.CallerID)
		if seed == "" && a.DeepSeekToken != "" {
			sum := sha256.Sum256([]byte(a.DeepSeekToken))
			seed = hex.EncodeToString(sum[:])
		}
	}
	headers := dsprotocol.BaseHeadersForRangersSeed(seed)
	if a == nil {
		return headers
	}
	profile := a.Account
	if seed != "" {
		if profile.Identifier() == "" {
			profile.Email = seed
		}
		profile = config.StableAndroidProfile(profile)
	}
	if value := strings.TrimSpace(profile.DeviceModel); value != "" {
		headers["x-device-model"] = value
	}
	if value := strings.TrimSpace(a.Account.RangersID); value != "" {
		headers["x-rangers-id"] = value
	}
	// Only forward real, unexpired HIF material. Missing/expired attestations
	// remain absent, as in the APK; these are not locally generated fingerprints.
	now := time.Now().Unix()
	if a.Account.HIFDliq != "" && a.Account.HIFDliqExpiresAt > now {
		headers["x-hif-dliq"] = a.Account.HIFDliq
	}
	if a.Account.HIFLeim != "" && a.Account.HIFLeimExpiresAt > now {
		headers["x-hif-leim"] = a.Account.HIFLeim
	}
	if profile.AndroidAPILevel != "" {
		headers["User-Agent"] = androidAPILevelPattern.ReplaceAllString(headers["User-Agent"], "Android/"+profile.AndroidAPILevel)
	}
	if a.Account.Locale != "" {
		headers["x-client-locale"] = a.Account.Locale
	}
	if a.Account.Timezone != "" {
		if loc, err := loadCachedLocation(a.Account.Timezone); err == nil {
			_, offset := time.Now().In(loc).Zone()
			headers["x-client-timezone-offset"] = strconv.Itoa(offset)
		}
	}
	deviceID := strings.TrimSpace(a.Account.InstallationID)
	if deviceID == "" {
		deviceID = strings.TrimSpace(headers["x-device-id"])
	}
	if deviceID == "" && seed != "" {
		deviceID = config.AndroidInstallationID(profile)
	}
	if deviceID != "" {
		headers["x-device-id"] = deviceID
	}
	return headers
}

func (c *Client) authHeadersFromContext(ctx context.Context, token string) map[string]string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.DeepSeekToken == token {
		return c.authHeadersForAuth(a)
	}
	return c.authHeaders(token)
}
