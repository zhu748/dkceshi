package toolcall

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"
	"sync"
)

// EPSEKeyword is the canonical, internal tool-call tag prefix. All prompt
// rendering, parsing and repair logic inside the repository continues to speak
// this fixed keyword. Per-deployment/per-key randomization happens only at the
// prompt boundary: the renderers emit a caller-specific marker and the output
// normalizer rewrites that marker back to EPSEKeyword before the sieve/parser
// see it.
const EPSEKeyword = "EPSE"

// markerAlphabet deliberately excludes look-alike characters (0/O, 1/I) so a
// marker is easy for the model to reproduce verbatim.
const markerAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const markerLength = 6

// DeriveToolMarker returns a stable, unpredictable marker for a caller.
//
// It is deterministic in (secret, callerID): the same API key always maps to
// the same marker, so multi-turn prompts and history re-rendering stay
// consistent. Because it is an HMAC keyed by a server-side secret, an observer
// (including the upstream model provider, or a caller in direct-token mode who
// knows their own token) cannot predict other callers' markers even though the
// derivation algorithm is public.
//
// When secret is empty the derivation falls back to a fixed internal salt. That
// still yields per-caller distinct markers, but they become predictable to
// anyone who knows the key; deployments should configure a secret.
//
// The returned marker satisfies two invariants, re-deriving with an incrementing
// salt until both hold: it never contains the canonical keyword, and it never
// overlaps itself (no proper suffix is also a proper prefix). The second one
// lets MarkerNormalizer reassemble a marker split across SSE chunk boundaries
// without ambiguity — see markerBorderLen.
func DeriveToolMarker(secret, callerID string) string {
	callerID = strings.TrimSpace(callerID)
	if callerID == "" {
		callerID = "anonymous"
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		secret = "ds2api-tool-marker-fallback"
	}

	// Re-derive with an incrementing salt until the marker satisfies both
	// invariants below. The first attempt (salt 0) is the common case, so the
	// published marker for a given caller is stable.
	//
	//   - It must not contain the canonical keyword, keeping caller and internal
	//     protocol prefixes unambiguous.
	//   - It must not overlap itself (see markerBorderLen): a bordered marker
	//     complicated older stream normalizers. Keep derived identities stable;
	//     the current normalizer also accepts legacy bordered markers.
	for attempt := 0; attempt < markerDeriveMaxAttempts; attempt++ {
		marker := deriveToolMarkerAttempt(secret, callerID, attempt)
		if !strings.Contains(marker, EPSEKeyword) && markerBorderLen(marker) == 0 {
			return marker
		}
	}
	// Unreachable in practice (each attempt is an independent 6-char draw with
	// a ~96.9% acceptance rate, so all eight failing is ~1e-12); the fallback
	// keeps both invariants without looping.
	return markerFallback
}

// markerDeriveMaxAttempts bounds the re-derivation loop in DeriveToolMarker.
const markerDeriveMaxAttempts = 8

// markerFallback is the last-resort marker. It contains no occurrence of
// EPSEKeyword and has no self-overlap.
const markerFallback = "Z2Q7ZK"

// markerBorderLen returns the length of the longest proper prefix of marker
// that is also a suffix of marker, or 0 when marker does not overlap itself.
//
// Derivation excludes bordered markers (for example "N3KZ5N") to preserve the
// existing identity algorithm. The stream normalizer also handles them when
// reading legacy markers by retaining whole matches until their boundary.
func markerBorderLen(marker string) int {
	for l := len(marker) - 1; l > 0; l-- {
		if marker[len(marker)-l:] == marker[:l] {
			return l
		}
	}
	return 0
}

func deriveToolMarkerAttempt(secret, callerID string, attempt int) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(callerID))
	if attempt > 0 {
		mac.Write([]byte{0x1f, byte(attempt)})
	}
	sum := mac.Sum(nil)

	var b strings.Builder
	b.Grow(markerLength)
	for i := 0; i < markerLength; i++ {
		b.WriteByte(markerAlphabet[int(sum[i])%len(markerAlphabet)])
	}
	return b.String()
}

// MarkerNormalizer rewrites caller-specific protocol prefixes to EPSEKeyword
// in model output. Ordinary occurrences and CDATA literal values are preserved.
//
// It is stateful because SSE deltas can split the marker at any byte: a chunk
// may end with "<|Q7Z" and only carry "K3M|tool_calls>" in the next one. The
// normalizer therefore holds back a trailing proper prefix of the marker until
// the next Write, so the sieve always sees a complete EPSE tag (or none).
//
// The zero value is a no-op normalizer, which is what callers get for the
// canonical EPSE marker.
type MarkerNormalizer struct {
	marker     string
	buf        string
	tagPrefix  bool
	inCDATA    bool
	cdataOpen  int
	cdataClose int
}

// NewMarkerNormalizer builds a normalizer for marker. Empty or canonical
// ("EPSE") markers yield a no-op normalizer.
func NewMarkerNormalizer(marker string) *MarkerNormalizer {
	m := strings.ToUpper(strings.TrimSpace(marker))
	if m == "" || m == EPSEKeyword {
		return &MarkerNormalizer{}
	}
	return &MarkerNormalizer{marker: m}
}

// Active reports whether this normalizer will rewrite anything.
func (n *MarkerNormalizer) Active() bool {
	return n != nil && n.marker != ""
}

// Write consumes the next stream chunk and returns the text that is safe to
// forward to the sieve. A trailing fragment that could still grow into the
// marker is withheld until the following Write (or Flush).
//
// The hold is measured on the text AFTER the last complete occurrence instead of
// on the whole buffer. A complete occurrence always ends in the marker itself,
// so measuring the whole buffer would let the occurrence's own tail pass for the
// head of the next occurrence whenever the marker overlaps itself ("N3KZ5N"):
// the occurrence would then be split, its head released un-rewritten (leaking
// the raw marker into visible text) and its tail withheld. Anchoring the hold
// after the last occurrence keeps every complete occurrence inside the released
// region; the context-aware rewriter then checks whether it is a tag prefix.
func (n *MarkerNormalizer) Write(chunk string) string {
	if !n.Active() {
		return chunk
	}
	// Fast path: with an empty hold buffer, no CDATA section open, no pending
	// tag prefix and no partial CDATA opener, a plain-ASCII chunk without '<'
	// cannot change the state machine (tag prefixes, CDATA openers and the
	// full-width variants observeRune normalizes all involve either '<' or a
	// non-ASCII byte), cannot contain a rewritable occurrence, and — when it
	// also does not end on a proper prefix of the marker — leaves nothing to
	// withhold. Ordinary prose therefore skips the occurrence scan and the
	// per-rune rewrite, which otherwise dominate the marker-normalization cost
	// on streaming deltas.
	if n.buf == "" && !n.inCDATA && !n.tagPrefix && n.cdataOpen == 0 &&
		plainASCIIWithoutLessThan(chunk) && trailingMarkerPrefixLen(chunk, n.marker) == 0 {
		return chunk
	}
	data := n.buf + chunk
	n.buf = ""
	hold := trailingMarkerPrefixLen(data[lastMarkerOccurrenceEnd(data, n.marker):], n.marker)
	n.buf = data[len(data)-hold:]
	out, tail := n.rewrite(data[:len(data)-hold], false)
	n.buf = tail + n.buf
	return out
}

// Flush releases any withheld fragment at end of stream.
func (n *MarkerNormalizer) Flush() string {
	if !n.Active() {
		return ""
	}
	out, _ := n.rewrite(n.buf, true)
	n.buf = ""
	return out
}

// NormalizeMarkerText normalizes tag prefixes without rewriting literal data.
// It is the non-streaming counterpart of MarkerNormalizer.
func NormalizeMarkerText(text, marker string) string {
	n := NewMarkerNormalizer(marker)
	if !n.Active() {
		return text
	}
	return n.Write(text) + n.Flush()
}

// ApplyToolMarker renders generated protocol tags, leaving ordinary text and
// literal CDATA arguments intact. Call this before merging with user content.
func ApplyToolMarker(text, marker string) string {
	marker = strings.TrimSpace(marker)
	if marker == "" || marker == EPSEKeyword || text == "" {
		return text
	}
	return replaceToolMarkupMarker(text, EPSEKeyword, marker)
}

// NormalizeToolMarkupMarker normalizes tags in serialized history without
// changing literal argument strings. It only accepts the exact generated tag
// delimiters, while model-output normalization also tolerates delimiter drift.
func NormalizeToolMarkupMarker(text, marker string) string {
	marker = strings.TrimSpace(marker)
	if marker == "" || marker == EPSEKeyword {
		return text
	}
	return replaceToolMarkupMarker(text, marker, EPSEKeyword)
}

// replacerCache memoizes the marker Replacers: ApplyToolMarker and
// NormalizeToolMarkupMarker run over every history message of every request,
// and strings.NewReplacer construction is pure overhead on that path. The
// cache is keyed by the (from,to) marker pair; distinct per-caller markers
// make the entry set small (one per active API key) and unbounded growth is
// capped by key count.
var replacerCache sync.Map // string -> *strings.Replacer

func markerReplacer(from, to string) *strings.Replacer {
	key := from + "\x00" + to
	if cached, ok := replacerCache.Load(key); ok {
		return cached.(*strings.Replacer)
	}
	replacer := strings.NewReplacer("<|"+from+"|", "<|"+to+"|", "</|"+from+"|", "</|"+to+"|")
	replacerCache.Store(key, replacer)
	return replacer
}

func replaceToolMarkupMarker(text, from, to string) string {
	if !strings.Contains(text, from) {
		return text
	}
	replacer := markerReplacer(from, to)
	// CDATA is literal tool argument content. Never rewrite user strings in it.
	var out strings.Builder
	for len(text) > 0 {
		start := strings.Index(text, "<![CDATA[")
		if start < 0 {
			out.WriteString(replacer.Replace(text))
			break
		}
		out.WriteString(replacer.Replace(text[:start]))
		end := strings.Index(text[start+9:], "]]>")
		if end < 0 {
			out.WriteString(text[start:])
			break
		}
		end += start + 12
		out.WriteString(text[start:end])
		text = text[end:]
	}
	return out.String()
}

// trailingMarkerPrefixLen returns the length of the longest suffix of data that
// is a proper prefix of marker (case-insensitive ASCII), capped at
// len(marker)-1.
//
// The scan is occurrence-agnostic: for a self-overlapping marker the tail of a
// complete occurrence is itself a proper prefix of the marker. Callers must
// therefore pass the text that FOLLOWS the last complete occurrence (see
// MarkerNormalizer.Write) so that a complete marker is never held back.
func trailingMarkerPrefixLen(data, marker string) int {
	max := len(marker) - 1
	if max > len(data) {
		max = len(data)
	}
	for l := max; l > 0; l-- {
		if asciiEqualFold(data[len(data)-l:], marker[:l]) {
			return l
		}
	}
	return 0
}

// lastMarkerOccurrenceEnd returns the offset just past the last complete marker
// occurrence in data, or 0 when data carries no complete occurrence. It uses the
// same left-to-right greedy scan as the marker buffer (a match consumes
// len(marker) bytes), so the returned offset is a boundary the rewriter agrees
// with.
func lastMarkerOccurrenceEnd(data, marker string) int {
	if marker == "" || len(data) < len(marker) {
		return 0
	}
	end := 0
	for i := 0; i+len(marker) <= len(data); {
		if asciiEqualFold(data[i:i+len(marker)], marker) {
			i += len(marker)
			end = i
			continue
		}
		i++
	}
	return end
}

func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

// plainASCIIWithoutLessThan reports whether s is pure ASCII and contains no
// '<'. Such a chunk cannot advance any MarkerNormalizer state: the '<', ']',
// '>' and slash transitions all need their ASCII form (full-width variants
// are non-ASCII), and separators/identifiers never set state on their own.
func plainASCIIWithoutLessThan(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 || s[i] == '<' {
			return false
		}
	}
	return true
}
