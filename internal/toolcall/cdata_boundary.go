package toolcall

import "strings"

// The CDATA opener immediately follows a parameter tag (allowing whitespace).
// Inspect lexical tags only: using the ignored-section scanner here recurses.
func cdataInParameter(text string, from int) bool {
	open := strings.LastIndexAny(text[:from], "<＜﹤〈")
	if open < 0 {
		return false
	}
	head := strings.TrimRight(text[:open], " \t\r\n")
	start := strings.LastIndexAny(head, "<＜﹤〈")
	if start < 0 {
		return false
	}
	tag, ok := scanToolMarkupTagAt(head, start)
	return ok && !tag.Closing && tag.Name == "parameter" && tag.End+1 == len(head)
}

func findToolCDATAEnd(text string, from int) int {
	if from < 0 || from >= len(text) {
		return -1
	}
	// Only a parameter value may use the legacy fence-aware tolerance. A
	// display literal ends at its own first close, regardless of later calls.
	if !cdataInParameter(text, from) {
		return indexToolCDATAClose(text, from)
	}
	firstNonFenceEnd := -1
	// Direction 3: `]]>` is a hard CDATA boundary. An odd number of markdown
	// fences inside the CDATA body (e.g. an unclosed ``` opener in file-writing
	// / code parameters) makes cdataOffsetIsInsideMarkdownFence report "inside
	// fence" for the whole body and skip the only structural `]]>`, so the real
	// close would be lost. When the body carries no tool-markup sample (plain
	// code / text, not a fenced tool-call example), fall back to the first
	// structural `]]>` so the hard boundary survives. When the body DOES carry
	// tool-markup (a fenced tool-call example), keep the conservative
	// fence-aware path so a mid-stream buffer does not mistake an example `]]>`
	// for the real close.
	firstStructuralEnd := -1
	bodyHasToolMarkup := toolMarkupPresent(text[from:])
	for searchFrom := from; searchFrom < len(text); {
		end := indexToolCDATAClose(text, searchFrom)
		if end < 0 {
			break
		}
		closeLen := toolCDATACloseLenAt(text, end)
		searchFrom = end + closeLen
		structural := cdataEndLooksStructural(text, searchFrom)
		if structural && firstStructuralEnd < 0 {
			firstStructuralEnd = end
		}
		if cdataOffsetIsInsideMarkdownFence(text[from:end]) {
			continue
		}
		if structural {
			return end
		}
		if firstNonFenceEnd < 0 {
			firstNonFenceEnd = end
		}
	}
	if firstNonFenceEnd >= 0 {
		return firstNonFenceEnd
	}
	if !bodyHasToolMarkup {
		return firstStructuralEnd
	}
	return -1
}
