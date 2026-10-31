package audio

import (
	"strings"
	"unicode"
)

// splitTTSText splits input on sentence boundaries so each segment stays under
// maxRunes. A single sentence longer than the limit falls back to clause
// boundaries and finally to a hard cut.
func splitTTSText(input string, maxRunes int) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = maxTTSSegmentRunes
	}
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return []string{input}
	}
	var segments []string
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		cut := -1
		for i := end - 1; i >= start; i-- {
			if isTTSSentenceEnd(runes[i]) {
				cut = i + 1
				break
			}
		}
		if cut <= start {
			for i := end - 1; i >= start; i-- {
				if isTTSClauseEnd(runes[i]) {
					cut = i + 1
					break
				}
			}
		}
		if cut > start {
			// Absorb immediately following closing quotes and brackets into the current segment.
			for cut < len(runes) && isTTSClosingPunctuation(runes[cut]) {
				cut++
			}
		} else {
			cut = end
			if cut < len(runes) && !unicode.IsSpace(runes[cut]) {
				// Backward search for a whitespace to prevent bisecting a word.
				for s := cut - 1; s > start; s-- {
					if unicode.IsSpace(runes[s]) {
						cut = s + 1
						break
					}
				}
			}
		}
		if segment := strings.TrimSpace(string(runes[start:cut])); segment != "" {
			segments = append(segments, segment)
		}
		start = cut
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}
	return segments
}

func isTTSClosingPunctuation(r rune) bool {
	switch r {
	case '”', '’', '"', '\'', '）', ')', '》', ']', '】', '』', '」':
		return true
	}
	return false
}

func isTTSSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '．', '\n':
		return true
	}
	return false
}

func isTTSClauseEnd(r rune) bool {
	switch r {
	case '，', '、', ',', '：', ':', '—', '…':
		return true
	}
	return false
}
