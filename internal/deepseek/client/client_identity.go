package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"

	"ds2api/internal/auth"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

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
	if value := strings.TrimSpace(a.Account.DeviceModel); value != "" {
		headers["x-device-model"] = value
	}
	if value := strings.TrimSpace(a.Account.RangersID); value != "" {
		headers["x-rangers-id"] = value
	}
	if a.Account.AndroidAPILevel != "" {
		headers["User-Agent"] = regexp.MustCompile(`Android/[0-9]+`).ReplaceAllString(headers["User-Agent"], "Android/"+a.Account.AndroidAPILevel)
	}
	if a.Account.Locale != "" {
		headers["x-client-locale"] = a.Account.Locale
	}
	deviceID := strings.TrimSpace(a.Account.DeviceID)
	if deviceID == "" {
		deviceID = strings.TrimSpace(headers["x-device-id"])
	}
	if deviceID == "" && seed != "" {
		// Local compatibility fallback, not an attested device-profile response.
		// A genuine installation's device_id should be supplied on the account.
		sum := sha256.Sum256([]byte("ds2api:deepseek:android:" + strings.ToLower(seed)))
		deviceID = base64.StdEncoding.EncodeToString(sum[:])
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
