package toolcall

import "strings"

// Only whitespace and complete comments may separate structural children.
// Unknown/misspelled children, truncated nodes and stray text carry information
// that must reach repair instead of disappearing from an executable call.
func emptyToolMarkupGap(text string) bool {
	for text = strings.TrimSpace(text); text != ""; text = strings.TrimSpace(text) {
		if !strings.HasPrefix(text, "<!--") {
			return false
		}
		end := strings.Index(text[4:], "-->")
		if end < 0 {
			return false
		}
		text = text[4+end+3:]
	}
	return true
}

func completeToolCallWrapper(body string) bool {
	invokes := findXMLElementBlocks(body, "invoke")
	if len(invokes) == 0 {
		return false
	}
	prev := 0
	for _, invoke := range invokes {
		if !emptyToolMarkupGap(body[prev:invoke.Start]) {
			return false
		}
		if _, ok := parseSingleXMLToolCall(invoke); !ok {
			return false
		}
		prev = invoke.End
	}
	return emptyToolMarkupGap(body[prev:])
}

func looksLikeToolCallSyntax(text string) bool {
	hasEPSE, hasCanonical := ContainsToolCallWrapperSyntaxOutsideIgnored(text)
	return hasEPSE || hasCanonical
}
