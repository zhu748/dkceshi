package sse

import "strings"

const minContinuationSnapshotLen = 32

func TrimContinuationOverlap(existing, incoming string) string {
	if incoming == "" {
		return ""
	}
	if existing == "" || !isContinuationSnapshot(incoming) {
		return incoming
	}
	if len(incoming) > len(existing) {
		if strings.HasPrefix(incoming, existing) {
			return incoming[len(existing):]
		}
		return incoming
	}
	if strings.HasPrefix(existing, incoming) {
		return ""
	}
	return incoming
}

func TrimContinuationOverlapFromBuilder(existing *strings.Builder, incoming string) string {
	if existing == nil {
		return incoming
	}
	return TrimContinuationOverlap(existing.String(), incoming)
}

// Only the threshold matters; counting an entire multi-megabyte snapshot on
// every delta adds a full scan before the prefix comparison can even begin.
func isContinuationSnapshot(text string) bool {
	count := 0
	for range text {
		count++
		if count >= minContinuationSnapshotLen {
			return true
		}
	}
	return false
}
