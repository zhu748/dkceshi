package transport

import (
	"strings"

	"github.com/sardanioss/httpcloak/fingerprint"
)

// ChromeMajorVersion is the Chrome generation advertised at the HTTP layer.
//
// It is the single source the devid runner consults when picking the version
// the browser env advertises (DS2API_DEVID_CHROME_MAJOR feeds the VM's UA,
// sec-ch-ua brands and navigator.userAgentData). Keep it in step with
// ChromePresetName: the preset carries the on-wire identity, this constant the
// version the payload and headers claim, and the two drifting apart is exactly
// the "UA says one thing, handshake says another" contradiction a risk engine
// can check inside a single request.
//
// Chrome ships roughly every four weeks and the overwhelming majority of real
// users sit within a few versions of latest, so a stale User-Agent is itself an
// anomaly. This is ported from ds2api's chrome-150 line (captured from the
// chat.deepseek.com web client generation), which also matches the httpcloak
// preset's own HTTP-layer claim below.
const ChromeMajorVersion = "150"

// ChromePresetName is the httpcloak browser preset every SDK fingerprint
// report request is dialed with. It supplies the TLS ClientHello, the HTTP/2
// framing fingerprint, and the preset's default browser headers.
//
// dkceshi 的数美身份是 Android Chrome（与账号的 Android 锚定画像同构），所以
// 预设取 android 家族：chrome-150-android 的 HTTP 层声称 Chrome 150 /
// "Android 10; K" / sec-ch-ua-mobile ?1，ClientHello 基于 148-android 抓包并
// 补 150 的 ML-DSA 签名算法。不要换成桌面预设——那会把“UA 说 Android、握手
// 却是 Windows Chrome”的矛盾写进同一个请求。移植自 ds2api 的
// chrome-150-windows 方案（httpcloak 同源指纹栈），预设家族按本项目身份选型。
//
// Keep ChromePresetName and ChromeMajorVersion in lockstep: the preset name is
// the HTTP-layer version, while its TLS base can lag a few versions because
// uTLS only models handshakes that were actually captured. Never pin the TLS
// generation as a second constant — resolve it from the preset via
// ChromeClientHelloName. Pinning it is exactly the drift that leaves a stale
// "TLS is Chrome 133" claim behind after the transport moves to a newer preset.
const ChromePresetName = "chrome-150-android"

// chromeSecChUa is the sec-ch-ua tuple for the claimed Chrome generation, taken
// from httpcloak's chrome-150 preset capture (GREASE brand first, then
// Chromium, then Google Chrome; the GREASE tuple and brand order rotate every
// major). The SDK env builds navigator.userAgentData.brands from the same
// string, so the report headers and the fingerprint payload never disagree.
var chromeSecChUa = `"Not;A=Brand";v="8", "Chromium";v="150", "Google Chrome";v="150"`

// SDKReportChromeMajor returns the Chrome major version the SDK report path
// claims at the HTTP layer. It is the value the devid runner forwards to the
// browser env (DS2API_DEVID_CHROME_MAJOR) so UA, sec-ch-ua brands and the
// transport preset never disagree inside one request.
func SDKReportChromeMajor() string {
	return ChromeMajorVersion
}

// SDKReportSecChUa returns the captured sec-ch-ua tuple matching
// SDKReportChromeMajor. Only pass it to the env when the claim is the same
// major (SDKReportChromeMajor); other generations must build their tuple from
// their own capture, not reuse this one.
func SDKReportSecChUa() string {
	return chromeSecChUa
}

// ChromeClientHelloName resolves the TLS ClientHello the configured preset
// actually reproduces, as its canonical httpcloak name (e.g.
// "chrome-148-android"). Reading it from the preset keeps the answer honest
// across preset bumps.
func ChromeClientHelloName() (string, bool) {
	preset := fingerprint.GetStrict(ChromePresetName)
	if preset == nil {
		return "", false
	}
	return fingerprint.ClientHelloIDName(preset.ClientHelloID)
}

// ChromeClientHelloMajor returns just the major version of the resolved TLS
// ClientHello (e.g. "148"), or false when it cannot be derived.
func ChromeClientHelloMajor() (string, bool) {
	name, ok := ChromeClientHelloName()
	if !ok {
		return "", false
	}
	major, _, _ := strings.Cut(strings.TrimPrefix(name, "chrome-"), "-")
	if major == "" {
		return "", false
	}
	return major, true
}

// chromeHeaderOrder pins the request header order. httpcloak applies it on top
// of its browser preset so every request reaches the SDK endpoint with a
// stable, Chrome-like header order.
//
// Names must be lowercase — httpcloak lowercases keys before looking them up in
// the order map, and headers absent from this list are appended afterwards in
// lexicographic order (still deterministic).
//
// This mirrors commonly observed Chrome fetch/XHR ordering, ported from ds2api
// (its chrome-150-windows transport). Headers the SDK report never carries
// simply do not match, so the DeepSeek-specific x-headers are harmless to keep
// listed for a day the order list is shared with upstream API requests. It is
// worth re-validating against a live capture if the upstream web client
// changes which headers it sets.
var chromeHeaderOrder = []string{
	"content-length",
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"sec-ch-ua-platform",
	"authorization",
	"x-client-bundle-id",
	"x-client-locale",
	"x-client-platform",
	"x-client-timezone-offset",
	"x-client-version",
	"x-device-id",
	"x-device-model",
	"x-ds-pow-response",
	"x-file-size",
	"x-hif-dliq",
	"x-hif-leim",
	"x-model-type",
	"x-request-encryption-key",
	"x-settings-token",
	"x-thinking-enabled",
	"user-agent",
	"content-type",
	"accept",
	"origin",
	"sec-fetch-site",
	"sec-fetch-mode",
	"sec-fetch-dest",
	"referer",
	"accept-encoding",
	"accept-language",
	"cookie",
	"priority",
}

// ChromeHeaderOrder returns a copy of the pinned request header order, for
// tooling that needs to reproduce or diff the exact on-wire ordering.
func ChromeHeaderOrder() []string {
	return append([]string(nil), chromeHeaderOrder...)
}

// ChromePseudoHeaderOrder returns a copy of the pinned HTTP/2 pseudo-header
// order.
func ChromePseudoHeaderOrder() []string {
	return append([]string(nil), chromePseudoHeaderOrder...)
}

// chromePseudoHeaderOrder is Chrome's :method,:authority,:scheme,:path order.
var chromePseudoHeaderOrder = []string{":method", ":authority", ":scheme", ":path"}
