package sse

import "strings"

func filterLeakedContentFilterParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return parts
	}
	out := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		cleaned, stripped := stripLeakedContentFilterSuffix(p.Text)
		// Only drop the chunk when we actually stripped a leaked CONTENT_FILTER
		// suffix. Plain whitespace chunks are valid SSE content and must stay.
		if stripped && shouldDropCleanedLeakedChunk(cleaned) {
			continue
		}
		if stripped {
			p.Text = cleaned
		}
		out = append(out, p)
	}
	return out
}

// leakedContentFilterProbe is the first byte (both cases) of the leaked
// marker we scan for; it gates the case-insensitive search so ordinary prose
// chunks pay a single bytes.IndexByte instead of a full ToUpper copy.
const leakedContentFilterMarker = "CONTENT_FILTER"

func stripLeakedContentFilterSuffix(text string) (string, bool) {
	if text == "" {
		return text, false
	}
	idx := indexCaseInsensitive(text, leakedContentFilterMarker)
	if idx < 0 {
		return text, false
	}
	// Keep "\n" so we don't collapse line structure when the upstream model
	// appends leaked CONTENT_FILTER markers after a line break.
	return strings.TrimRight(text[:idx], " \t\r"), true
}

// indexCaseInsensitive finds the first case-insensitive ASCII occurrence of
// substr in s without allocating an uppercased copy of s. It only ever runs on
// chunks that contain the marker's first letter, so the byte-by-byte compare
// below is rarely reached in full.
func indexCaseInsensitive(s, substr string) int {
	if substr == "" || len(s) < len(substr) {
		return -1
	}
	first := substr[0]
	lowerFirst := first | 0x20 // ASCII lowercase of the first byte
	for i := 0; i+len(substr) <= len(s); i++ {
		c := s[i]
		if c != first && c != lowerFirst {
			continue
		}
		if asciiFoldEqual(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func asciiFoldEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca |= 0x20
		}
		if cb >= 'A' && cb <= 'Z' {
			cb |= 0x20
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func shouldDropCleanedLeakedChunk(cleaned string) bool {
	if cleaned == "" {
		return true
	}
	// Preserve newline-only chunks to avoid dropping legitimate line breaks
	// before a leaked CONTENT_FILTER suffix.
	if strings.Contains(cleaned, "\n") {
		return false
	}
	return strings.TrimSpace(cleaned) == ""
}
