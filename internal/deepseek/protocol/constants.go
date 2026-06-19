package protocol

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	DeepSeekHost                 = "chat.deepseek.com"
	DeepSeekLoginURL             = "https://chat.deepseek.com/api/v0/users/login"
	DeepSeekCreateSessionURL     = "https://chat.deepseek.com/api/v0/chat_session/create"
	DeepSeekCreatePowURL         = "https://chat.deepseek.com/api/v0/chat/create_pow_challenge"
	DeepSeekCompletionURL        = "https://chat.deepseek.com/api/v0/chat/completion"
	DeepSeekContinueURL          = "https://chat.deepseek.com/api/v0/chat/continue"
	DeepSeekUploadFileURL        = "https://chat.deepseek.com/api/v0/file/upload_file"
	DeepSeekFetchFilesURL        = "https://chat.deepseek.com/api/v0/file/fetch_files"
	DeepSeekFetchSessionURL      = "https://chat.deepseek.com/api/v0/chat_session/fetch_page"
	DeepSeekDeleteSessionURL     = "https://chat.deepseek.com/api/v0/chat_session/delete"
	DeepSeekDeleteAllSessionsURL = "https://chat.deepseek.com/api/v0/chat_session/delete_all"
	DeepSeekCompletionTargetPath = "/api/v0/chat/completion"
	DeepSeekUploadTargetPath     = "/api/v0/file/upload_file"
)

var defaultStaticBaseHeaders = map[string]string{
	"Host":           "chat.deepseek.com",
	"Accept":         "application/json",
	"Content-Type":   "application/json",
	"accept-charset": "UTF-8",
}

var defaultSkipContainsPatterns = []string{
	"quasi_status",
	"elapsed_secs",
	"token_usage",
	"pending_fragment",
	"conversation_mode",
	"fragments/-1/status",
	"fragments/-2/status",
	"fragments/-3/status",
}

var defaultSkipExactPaths = []string{
	"response/search_status",
}

var ClientVersion string
var BaseHeaders = map[string]string{}
var SkipContainsPatterns = cloneStringSlice(defaultSkipContainsPatterns)
var SkipExactPathSet = toStringSet(defaultSkipExactPaths)
var explicitRangersID string

type clientConstants struct {
	Name            string `json:"name"`
	Platform        string `json:"platform"`
	Version         string `json:"version"`
	AndroidAPILevel string `json:"android_api_level"`
	Locale          string `json:"locale"`
	TimezoneOffset  string `json:"timezone_offset"`
	RangersID       string `json:"rangers_id"`
}

type sharedConstants struct {
	Client              clientConstants   `json:"client"`
	BaseHeaders         map[string]string `json:"base_headers"`
	SkipContainsPattern []string          `json:"skip_contains_patterns"`
	SkipExactPaths      []string          `json:"skip_exact_paths"`
}

//go:embed constants_shared.json
var sharedConstantsJSON []byte

func init() {
	cfg := sharedConstants{}
	if err := json.Unmarshal(sharedConstantsJSON, &cfg); err != nil {
		panic(fmt.Errorf("load DeepSeek shared constants: %w", err))
	}
	applySharedConstants(cfg)
}

func applySharedConstants(cfg sharedConstants) {
	client := normalizeClientConstants(cfg.Client)
	client = applyEnvClientOverrides(client)
	ClientVersion = client.Version
	BaseHeaders = buildBaseHeaders(client, cfg.BaseHeaders)
	SkipContainsPatterns = cloneStringSlice(defaultSkipContainsPatterns)
	if len(cfg.SkipContainsPattern) > 0 {
		SkipContainsPatterns = cloneStringSlice(cfg.SkipContainsPattern)
	}
	SkipExactPathSet = toStringSet(defaultSkipExactPaths)
	if len(cfg.SkipExactPaths) > 0 {
		SkipExactPathSet = toStringSet(cfg.SkipExactPaths)
	}
}

func normalizeClientConstants(in clientConstants) clientConstants {
	if in.Name == "" {
		in.Name = "DeepSeek"
	}
	if in.Platform == "" {
		in.Platform = "android"
	}
	if in.AndroidAPILevel == "" {
		in.AndroidAPILevel = "35"
	}
	if in.Locale == "" {
		in.Locale = "zh_CN"
	}
	if in.TimezoneOffset == "" {
		in.TimezoneOffset = "28800"
	}
	if in.RangersID == "" {
		in.RangersID = deriveDefaultRangersID()
	}
	return in
}

func applyEnvClientOverrides(in clientConstants) clientConstants {
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_CLIENT_NAME")); v != "" {
		in.Name = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_CLIENT_PLATFORM")); v != "" {
		in.Platform = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_CLIENT_VERSION")); v != "" {
		in.Version = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_ANDROID_API_LEVEL")); v != "" {
		in.AndroidAPILevel = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_CLIENT_LOCALE")); v != "" {
		in.Locale = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_CLIENT_TIMEZONE_OFFSET")); v != "" {
		in.TimezoneOffset = v
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_DEEPSEEK_RANGERS_ID")); v != "" {
		in.RangersID = v
		explicitRangersID = v
	}
	return in
}

func deriveDefaultRangersID() string {
	seedParts := []string{
		os.Getenv("DS2API_DEEPSEEK_RANGERS_SEED"),
		os.Getenv("VERCEL_PROJECT_ID"),
		os.Getenv("RENDER_SERVICE_ID"),
		os.Getenv("RENDER_EXTERNAL_HOSTNAME"),
		os.Getenv("DS2API_CONFIG_PATH"),
	}
	if hostname, err := os.Hostname(); err == nil {
		seedParts = append(seedParts, hostname)
	}
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		seedParts = append(seedParts, userConfigDir)
	}
	seed := strings.Join(seedParts, "|")
	if strings.Trim(seed, "| ") == "" {
		seed = "ds2api:deepseek:android"
	}
	return deriveRangersIDFromSeed(seed)
}

func deriveRangersIDFromSeed(seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		seed = "ds2api:deepseek:android"
	}
	sum := sha256.Sum256([]byte(seed))
	n := binary.BigEndian.Uint64(sum[:8])%1_000_000_000_000_000_000 + 7_000_000_000_000_000_000
	return fmt.Sprintf("%d", n)
}

func BaseHeadersForRangersSeed(seed string) map[string]string {
	out := cloneStringMap(BaseHeaders)
	if explicitRangersID != "" {
		out["x-rangers-id"] = explicitRangersID
		return out
	}
	if strings.TrimSpace(seed) != "" {
		out["x-rangers-id"] = deriveRangersIDFromSeed("account:" + seed)
	}
	return out
}

func buildBaseHeaders(client clientConstants, overrides map[string]string) map[string]string {
	out := cloneStringMap(defaultStaticBaseHeaders)
	for k, v := range overrides {
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if client.Name != "" && client.Version != "" {
		userAgent := client.Name + "/" + client.Version
		if client.Platform == "android" && client.AndroidAPILevel != "" {
			userAgent += " Android/" + client.AndroidAPILevel
		}
		out["User-Agent"] = userAgent
	}
	if client.Platform != "" {
		out["x-client-platform"] = client.Platform
	}
	if client.Version != "" {
		out["x-client-version"] = client.Version
	}
	if client.Locale != "" {
		out["x-client-locale"] = client.Locale
		if _, ok := out["Accept-Language"]; !ok {
			out["Accept-Language"] = acceptLanguageFromLocale(client.Locale)
		}
	}
	if client.TimezoneOffset != "" {
		out["x-client-timezone-offset"] = client.TimezoneOffset
	}
	if client.RangersID != "" {
		out["x-rangers-id"] = client.RangersID
	}
	return out
}

func acceptLanguageFromLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}
	tag := strings.ReplaceAll(locale, "_", "-")
	base := tag
	if idx := strings.Index(base, "-"); idx >= 0 {
		base = base[:idx]
	}
	base = strings.TrimSpace(base)
	if base == "" || base == tag {
		return tag
	}
	return tag + "," + base + ";q=0.9"
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringSlice(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func toStringSet(in []string) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	return out
}

const (
	KeepAliveTimeout  = 5
	StreamIdleTimeout = 300
	MaxKeepaliveCount = 40
)
