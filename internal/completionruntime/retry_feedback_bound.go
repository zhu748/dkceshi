package completionruntime

import "unicode/utf8"

// Inspect only a bounded prefix and tail, even for multi-megabyte rejected
// arguments. Preserve small examples exactly and never split a UTF-8 rune.
func boundRetryFeedback(text string, limit int, keepTail bool) (string, bool) {
	const separator = "\n…[truncated]…\n"
	headChars, headEnd, count := limit/2, 0, 0
	for offset := range text {
		if count == headChars {
			headEnd = offset
		}
		if count == limit {
			if !keepTail {
				return text[:offset] + "…", true
			}
			tailChars := limit - headChars - utf8.RuneCountInString(separator)
			tail := len(text)
			for i := 0; i < tailChars; i++ {
				_, size := utf8.DecodeLastRuneInString(text[:tail])
				tail -= size
			}
			return text[:headEnd] + separator + text[tail:], true
		}
		count++
	}
	return text, false
}
