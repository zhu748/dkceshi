package toolcall

import "strings"

// Reasoning is allowed to recover a misplaced tool-only turn. Planning prose,
// quoted examples and comments alongside calls are not execution instructions.
func IsToolOnlySource(text string) bool {
	spans := BadToolMarkupSpans(text)
	for _, block := range findToolCallElementBlocksOutsideIgnored(text) {
		spans = append(spans, TextSpan{Start: block.Start, End: block.End})
	}
	return len(spans) > 0 && strings.TrimSpace(RemoveTextSpans(text, spans)) == ""
}
