package toolstream

import "ds2api/internal/toolcall"

func splitSafeContentForToolDetection(state *State, s string) (safe, hold string) {
	if s == "" {
		return "", ""
	}
	if xmlIdx := findPartialXMLToolTagStart(s); xmlIdx >= 0 {
		if insideCodeFenceWithState(state, s[:xmlIdx]) {
			return s, ""
		}
		markdown := markdownCodeSpanStateAt(state, s[:xmlIdx])
		if markdown.ticks > 0 {
			if markdownCodeSpanCloses(s[xmlIdx:], markdown.ticks) {
				return s, ""
			}
			if markdown.fromPrior {
				return "", s
			}
		}
		if xmlIdx > 0 {
			return s[:xmlIdx], s[xmlIdx:]
		}
		return "", s
	}
	return s, ""
}

const holdToolSegmentStart = -2

func findToolSegmentStart(state *State, s string) int {
	if s == "" {
		return -1
	}
	offset := 0
	for {
		start, end, ok := findNextToolInterceptTag(s, offset)
		if state != nil && state.DeferMalformed {
			if candidate, _ := toolcall.FindToolProtocolPrefixOutsideIgnored(s, offset); candidate >= 0 {
				if (!ok || candidate < start) && !insideCodeFenceWithState(state, s[:candidate]) && markdownCodeSpanStateAt(state, s[:candidate]).ticks == 0 {
					return candidate
				}
			}
		}
		if !ok {
			return -1
		}
		start = includeDuplicateLeadingLessThan(s, start)
		if insideCodeFenceWithState(state, s[:start]) {
			offset = end + 1
			continue
		}
		markdown := markdownCodeSpanStateAt(state, s[:start])
		if markdown.ticks == 0 {
			return start
		}
		if markdownCodeSpanCloses(s[start:], markdown.ticks) {
			offset = end + 1
			continue
		}
		if state != nil && state.DeferMalformed {
			return holdToolSegmentStart
		}
		tag, isToolTag := toolcall.FindToolMarkupTagOutsideIgnored(s, start)
		if markdown.fromPrior || (isToolTag && tag.Start == start && tag.Closing) {
			return holdToolSegmentStart
		}
		return start
	}
}

// findNextToolInterceptTag returns the earliest streaming interception entry at
// or after offset: a recognized EPSE / canonical tool markup tag, or a
// DSML-prefixed tag. DSML output never parses into a tool call, but its wrapper
// still has to be captured so the raw alien markup is not leaked piecemeal.
func findNextToolInterceptTag(s string, offset int) (start, end int, ok bool) {
	tag, tagOK := toolcall.FindToolMarkupTagOutsideIgnored(s, offset)
	dsml, dsmlOK := toolcall.FindDSMLTagOutsideIgnored(s, offset)
	switch {
	case tagOK && dsmlOK:
		if tag.Start <= dsml.Start {
			return tag.Start, tag.End, true
		}
		return dsml.Start, dsml.End, true
	case tagOK:
		return tag.Start, tag.End, true
	case dsmlOK:
		return dsml.Start, dsml.End, true
	default:
		return 0, 0, false
	}
}

// consumeDSMLHiddenCapture hides a DSML block from visible output:
// prefix is the text before the first DSML tag; suffix is the text after the
// matched closing tag. handled=true means a DSML tag was found; hold=true means
// the outer wrapper has not closed yet and the caller should keep buffering.
// When final is true an unclosed tail is discarded as well.
func consumeDSMLHiddenCapture(captured string, final bool) (prefix, suffix string, handled, hold bool) {
	open, ok := toolcall.FindDSMLTagOutsideIgnored(captured, 0)
	if !ok {
		return "", "", false, false
	}
	prefix = captured[:open.Start]
	closeTag, ok := toolcall.FindMatchingDSMLClose(captured, open)
	if !ok {
		if final {
			return prefix, "", true, false
		}
		return "", "", true, true
	}
	return prefix, captured[closeTag.End+1:], true, false
}

type markdownCodeSpanScan struct {
	ticks     int
	fromPrior bool
}

func markdownCodeSpanStateAt(state *State, text string) markdownCodeSpanScan {
	ticks := 0
	fromPrior := false
	if state != nil && state.markdownCodeSpanTicks > 0 {
		ticks = state.markdownCodeSpanTicks
		fromPrior = true
	}
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		run := countBacktickRun(text, i)
		if ticks == 0 {
			if run >= 3 && atMarkdownFenceLineStart(text, i) {
				i += run
				continue
			}
			if state != nil && insideCodeFenceWithState(state, text[:i]) {
				i += run
				continue
			}
			ticks = run
			fromPrior = false
		} else if run == ticks {
			ticks = 0
			fromPrior = false
		}
		i += run
	}
	return markdownCodeSpanScan{ticks: ticks, fromPrior: fromPrior}
}

func markdownCodeSpanCloses(text string, ticks int) bool {
	if ticks <= 0 {
		return false
	}
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}
		run := countBacktickRun(text, i)
		if run == ticks {
			return true
		}
		i += run
	}
	return false
}

func consumeToolCapture(state *State, toolNames []string, final bool) (prefix string, calls []toolcall.ParsedToolCall, suffix string, ready bool) {
	captured := state.capture.String()
	if captured == "" {
		return "", nil, "", false
	}
	if !final && len(captured) > 4096 && !state.captureProgress.ready(captured) {
		return "", nil, "", false
	}
	// Real blocks consume their closes atomically. A fresh leading close is
	// an orphan, not malformed invocation text awaiting the repair pass.
	if tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(captured, 0); ok && tag.Start == 0 && tag.Closing {
		return "", nil, captured[tag.End+1:], true
	}
	if tag, ok := toolcall.FindDSMLTagOutsideIgnored(captured, 0); ok && tag.Start == 0 && tag.Closing {
		return "", nil, captured[tag.End+1:], true
	}
	if state.DisplayOnly {
		if tag, ok := toolcall.FindToolMarkupTagOutsideIgnored(captured, 0); ok && tag.Start == 0 && tag.Name != "tool_calls" {
			if closeTag, ok := toolcall.FindMatchingToolMarkupClose(captured, tag); ok {
				return "", nil, captured[closeTag.End+1:], true
			}
		}
	}

	// XML tool call extraction only.
	if xmlPrefix, xmlCalls, xmlSuffix, xmlReady := consumeXMLToolCapture(captured, toolNames); xmlReady {
		if state.DeferMalformed && !final && len(xmlCalls) == 0 {
			return "", nil, "", false
		}
		return xmlPrefix, xmlCalls, xmlSuffix, true
	}
	// If XML tags are present but block is incomplete, keep buffering.
	if hasOpenXMLToolTag(captured) {
		return "", nil, "", false
	}
	if state.DeferMalformed && !final {
		if idx, _ := toolcall.FindToolProtocolPrefixOutsideIgnored(captured, 0); idx >= 0 {
			return "", nil, "", false
		}
	}
	// DSML never parses into a tool call: hide it from visible content while
	// keeping the raw text available for the finalize repair path.
	if dsmlPrefix, dsmlSuffix, handled, hold := consumeDSMLHiddenCapture(captured, final); handled {
		if hold {
			return "", nil, "", false
		}
		return dsmlPrefix, nil, dsmlSuffix, true
	}
	if shouldKeepBareInvokeCapture(captured) {
		return "", nil, "", false
	}
	return captured, nil, "", true
}

func shouldResetUnclosedMarkdownPrefix(state *State, prefix, suffix string) bool {
	markdown := markdownCodeSpanStateAt(state, prefix)
	return markdown.ticks > 0 && !markdown.fromPrior && !markdownCodeSpanCloses(suffix, markdown.ticks)
}
