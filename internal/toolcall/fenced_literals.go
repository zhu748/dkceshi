package toolcall

import "strings"

func stripFencedCodeBlocks(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(text))

	lines := strings.SplitAfter(text, "\n")
	inFence := false
	fenceMarker := ""
	inCDATA := false
	cdataFenceMarker := ""
	// Track builder length when a fence opens so we can preserve content
	// collected before the unclosed fence.
	beforeFenceLen := 0
	// Track the byte offset in the original text where the current unclosed
	// fence began, so the raw tail can be recovered when the fence turns out to
	// be a false positive (see the inFence block below).
	fenceStartOffset := -1
	lineStart := 0
	for _, line := range lines {
		curLineStart := lineStart
		lineStart += len(line)
		if !inFence && (inCDATA || cdataStartsBeforeFence(line)) {
			b.WriteString(line)
			inCDATA, cdataFenceMarker = updateCDATAStateForStrip(inCDATA, cdataFenceMarker, line)
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if marker, ok := parseFenceOpen(trimmed); ok {
				inFence = true
				fenceMarker = marker
				beforeFenceLen = b.Len()
				fenceStartOffset = curLineStart
				continue
			}
			b.WriteString(line)
			continue
		}

		if isFenceClose(trimmed, fenceMarker) {
			inFence = false
			fenceMarker = ""
		}
	}

	if inFence {
		// Unclosed fence: normally we drop the fenced region and keep only the
		// content collected before it. But tool-call parameters routinely carry
		// an odd number of markdown fences (```), which makes a real tool call
		// look like an unclosed fence and swallows the trailing tool close
		// tags. If the dropped tail still carries tool-structural closing tags,
		// treat the fence as a false positive and preserve the raw tail so the
		// close tags survive for intent detection and parsing.
		result := b.String()
		head := ""
		if beforeFenceLen > 0 && beforeFenceLen <= len(result) {
			head = result[:beforeFenceLen]
		}
		if fenceStartOffset >= 0 && fenceStartOffset <= len(text) {
			if tail := text[fenceStartOffset:]; openToolParameterBeforeFence(head) && textContainsToolMarkupCloseTag(tail) {
				return head + tail
			}
		}
		return head
	}
	return b.String()
}
