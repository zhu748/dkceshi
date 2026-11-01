package toolcall

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"encoding/xml"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var xmlAttrPattern = regexp.MustCompile(`(?is)\b([a-z0-9_:-]+)\s*=\s*("([^"]*)"|'([^']*)')`)
var cdataBRSeparatorPattern = regexp.MustCompile(`(?i)<br\s*/?>`)

func parseXMLToolCalls(text string) []ParsedToolCall {
	wrappers := findToolCallElementBlocksOutsideIgnored(text)
	if len(wrappers) == 0 {
		repaired := repairMissingXMLToolCallsOpeningWrapper(text)
		if repaired != text {
			wrappers = findToolCallElementBlocksOutsideIgnored(repaired)
		}
	}
	if len(wrappers) == 0 {
		return nil
	}
	out := make([]ParsedToolCall, 0, len(wrappers))
	for _, wrapper := range wrappers {
		for _, block := range findXMLElementBlocks(wrapper.Body, "invoke") {
			call, ok := parseSingleXMLToolCall(block)
			if !ok {
				continue
			}
			out = append(out, call)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func findToolCallElementBlocksOutsideIgnored(text string) []xmlElementBlock {
	return findToolMarkupElementBlocksByName(text, "tool_calls")
}

// findToolMarkupElementBlocksByName scans EPSE-aware markup for balanced blocks
// of the given local name (e.g. "tool_calls" or "invoke") outside CDATA /
// comments / fenced or inline code, returning each block's absolute Start/End
// and Body. Coordinates are absolute in text, so it can be run directly on the
// pre-normalization original to locate deduction regions (§4.2).
func findToolMarkupElementBlocksByName(text, name string) []xmlElementBlock {
	if text == "" || name == "" {
		return nil
	}
	var out []xmlElementBlock
	for searchFrom := 0; searchFrom < len(text); {
		tag, ok := FindToolMarkupTagOutsideIgnored(text, searchFrom)
		if !ok {
			break
		}
		if tag.Closing || tag.Name != name {
			searchFrom = tag.End + 1
			continue
		}
		closeTag, ok := FindMatchingToolMarkupClose(text, tag)
		if !ok {
			searchFrom = tag.End + 1
			continue
		}
		attrsEnd := tag.End + 1
		if delimLen := xmlTagEndDelimiterLenEndingAt(text, tag.End); delimLen > 0 {
			attrsEnd = tag.End + 1 - delimLen
		}
		out = append(out, xmlElementBlock{
			Attrs: text[tag.NameEnd:attrsEnd],
			Body:  text[tag.End+1 : closeTag.Start],
			Start: tag.Start,
			End:   closeTag.End + 1,
		})
		searchFrom = closeTag.End + 1
	}
	return out
}

func repairMissingXMLToolCallsOpeningWrapper(text string) string {
	if _, ok := firstToolMarkupTagByName(text, "tool_calls", false); ok {
		return text
	}

	invokeTag, ok := firstToolMarkupTagByName(text, "invoke", false)
	if !ok {
		return text
	}
	closeTag, ok := lastToolMarkupTagByName(text, "tool_calls", true)
	if !ok || invokeTag.Start >= closeTag.Start {
		return text
	}

	return text[:invokeTag.Start] + "<tool_calls>" + text[invokeTag.Start:closeTag.Start] + "</tool_calls>" + text[closeTag.End+1:]
}

func firstToolMarkupTagByName(text, name string, closing bool) (ToolMarkupTag, bool) {
	for searchFrom := 0; searchFrom < len(text); {
		tag, ok := FindToolMarkupTagOutsideIgnored(text, searchFrom)
		if !ok {
			break
		}
		if tag.Name == name && tag.Closing == closing {
			return tag, true
		}
		searchFrom = tag.End + 1
	}
	return ToolMarkupTag{}, false
}

func lastToolMarkupTagByName(text, name string, closing bool) (ToolMarkupTag, bool) {
	var last ToolMarkupTag
	found := false
	for searchFrom := 0; searchFrom < len(text); {
		tag, ok := FindToolMarkupTagOutsideIgnored(text, searchFrom)
		if !ok {
			break
		}
		if tag.Name == name && tag.Closing == closing {
			last = tag
			found = true
		}
		searchFrom = tag.End + 1
	}
	if !found {
		return ToolMarkupTag{}, false
	}
	return last, true
}

func parseSingleXMLToolCall(block xmlElementBlock) (ParsedToolCall, bool) {
	attrs := parseXMLTagAttributes(block.Attrs)
	name := strings.TrimSpace(html.UnescapeString(attrs["name"]))
	if name == "" {
		return ParsedToolCall{}, false
	}

	inner := strings.TrimSpace(block.Body)
	if strings.HasPrefix(inner, "{") {
		var payload map[string]any
		if err := jsonvalue.Unmarshal([]byte(inner), &payload); err == nil {
			return parseJSONToolCallEnvelope(name, payload)
		}
	}

	input := map[string]any{}
	literals := map[string]string{}
	prev := 0
	for _, paramMatch := range findXMLElementBlocks(inner, "parameter") {
		if !emptyToolMarkupGap(inner[prev:paramMatch.Start]) {
			return ParsedToolCall{}, false
		}
		paramAttrs := parseXMLTagAttributes(paramMatch.Attrs)
		paramName := strings.TrimSpace(html.UnescapeString(paramAttrs["name"]))
		if paramName == "" {
			return ParsedToolCall{}, false
		}
		value := parseInvokeParameterValue(paramName, paramMatch.Body)
		recordStringLiteral(literals, input, paramName, paramMatch.Body, value)
		appendMarkupValue(input, paramName, value)
		prev = paramMatch.End
	}
	if !emptyToolMarkupGap(inner[prev:]) {
		return ParsedToolCall{}, false
	}

	if len(input) == 0 {
		if strings.TrimSpace(inner) != "" {
			return ParsedToolCall{}, false
		}
		return ParsedToolCall{Name: name, Input: map[string]any{}}, true
	}
	if len(literals) == 0 {
		literals = nil
	}
	return ParsedToolCall{Name: name, Input: input, StringLiterals: literals}, true
}

type xmlElementBlock struct {
	Attrs string
	Body  string
	Start int
	End   int
}

func findXMLElementBlocks(text, tag string) []xmlElementBlock {
	if text == "" || tag == "" {
		return nil
	}
	var out []xmlElementBlock
	pos := 0
	for pos < len(text) {
		start, bodyStart, attrs, ok := findXMLStartTagOutsideCDATA(text, tag, pos)
		if !ok {
			break
		}
		closeStart, closeEnd, ok := findMatchingXMLEndTagOutsideCDATA(text, tag, bodyStart)
		if !ok {
			pos = bodyStart
			continue
		}
		out = append(out, xmlElementBlock{
			Attrs: attrs,
			Body:  text[bodyStart:closeStart],
			Start: start,
			End:   closeEnd,
		})
		pos = closeEnd
	}
	return out
}

func findXMLStartTagOutsideCDATA(text, tag string, from int) (start, bodyStart int, attrs string, ok bool) {
	target := "<" + strings.ToLower(tag)
	for i := maxInt(from, 0); i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return -1, -1, "", false
		}
		if advanced {
			i = next
			continue
		}
		if hasASCIIPrefixFoldAt(text, i, target) && hasXMLTagBoundary(text, i+len(target)) {
			end := findXMLTagEnd(text, i+len(target))
			if end < 0 {
				return -1, -1, "", false
			}
			return i, end + 1, text[i+len(target) : end], true
		}
		i++
	}
	return -1, -1, "", false
}

func findMatchingXMLEndTagOutsideCDATA(text, tag string, from int) (closeStart, closeEnd int, ok bool) {
	openTarget := "<" + strings.ToLower(tag)
	closeTarget := "</" + strings.ToLower(tag)
	depth := 1
	for i := maxInt(from, 0); i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return -1, -1, false
		}
		if advanced {
			i = next
			continue
		}
		if hasASCIIPrefixFoldAt(text, i, closeTarget) && hasXMLTagBoundary(text, i+len(closeTarget)) {
			end := findXMLTagEnd(text, i+len(closeTarget))
			if end < 0 {
				return -1, -1, false
			}
			depth--
			if depth == 0 {
				return i, end + 1, true
			}
			i = end + 1
			continue
		}
		if hasASCIIPrefixFoldAt(text, i, openTarget) && hasXMLTagBoundary(text, i+len(openTarget)) {
			end := findXMLTagEnd(text, i+len(openTarget))
			if end < 0 {
				return -1, -1, false
			}
			if !isSelfClosingXMLTag(text[:end]) {
				depth++
			}
			i = end + 1
			continue
		}
		i++
	}
	return -1, -1, false
}

func skipXMLIgnoredSection(text string, i int) (next int, advanced bool, blocked bool) {
	if i < 0 || i >= len(text) {
		return i, false, false
	}
	// JSON argument strings can contain literal tool markup. Skip only a
	// complete, valid JSON string; a stray prose quote must not hide the tail.
	if text[i] == '"' {
		for end := i + 1; end < len(text); end++ {
			if text[end] == '\\' {
				end++
			} else if text[end] == '"' {
				if json.Valid([]byte(text[i : end+1])) {
					return end + 1, true, false
				}
				break
			} else if text[end] < ' ' {
				break
			}
		}
	}
	if bodyStart, ok := matchToolCDATAOpenAt(text, i); ok {
		end := findToolCDATAEnd(text, bodyStart)
		if end < 0 {
			return 0, false, true
		}
		return end + toolCDATACloseLenAt(text, end), true, false
	}
	switch {
	case strings.HasPrefix(text[i:], "<!--"):
		end := strings.Index(text[i+len("<!--"):], "-->")
		if end < 0 {
			return 0, false, true
		}
		return i + len("<!--") + end + len("-->"), true, false
	default:
		return i, false, false
	}
}

func matchToolCDATAOpenAt(text string, start int) (int, bool) {
	openLen := toolCDATAOpenLenAt(text, start)
	if openLen > 0 {
		return start + openLen, true
	}
	return start, false
}

func hasASCIIPrefixFoldAt(text string, start int, prefix string) bool {
	_, ok := matchASCIIPrefixFoldAt(text, start, prefix)
	return ok
}

func matchASCIIPrefixFoldAt(text string, start int, prefix string) (int, bool) {
	if start < 0 || start >= len(text) && prefix != "" {
		return 0, false
	}
	idx := start
	for j := 0; j < len(prefix); j++ {
		if idx >= len(text) {
			return 0, false
		}
		ch, size := normalizedASCIIAt(text, idx)
		if size <= 0 || asciiLower(ch) != asciiLower(prefix[j]) {
			return 0, false
		}
		idx += size
	}
	return idx - start, true
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// toolMarkupPresent reports whether s contains a recognizable tool-call markup
// fragment (EPSE or canonical XML). It distinguishes a fenced tool-call example
// (which must keep the fence-aware CDATA path so an example `]]>` is not taken
// for the real close) from plain code/text that merely carries an odd markdown
// fence (where the structural `]]>` really is the close).
func toolMarkupPresent(s string) bool {
	for _, marker := range []string{
		"<|EPSE|", "<tool_calls", "<invoke", "<parameter", "<epse-",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func indexToolCDATAClose(text string, from int) int {
	if from < 0 {
		from = 0
	}
	asciiIdx := strings.Index(text[from:], "]]>")
	fullIdx := strings.Index(text[from:], "]]＞")
	cjkIdx := strings.Index(text[from:], "]]〉")
	if asciiIdx < 0 && fullIdx < 0 && cjkIdx < 0 {
		return -1
	}
	best := -1
	for _, idx := range []int{asciiIdx, fullIdx, cjkIdx} {
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	return from + best
}

func toolCDATACloseLenAt(text string, idx int) int {
	if idx < 0 || idx >= len(text) {
		return 0
	}
	if strings.HasPrefix(text[idx:], "]]〉") {
		return len("]]〉")
	}
	if strings.HasPrefix(text[idx:], "]]＞") {
		return len("]]＞")
	}
	if strings.HasPrefix(text[idx:], "]]>") {
		return len("]]>")
	}
	return 0
}

func cdataEndLooksStructural(text string, after int) bool {
	for after < len(text) {
		switch {
		case text[after] == ' ' || text[after] == '\t' || text[after] == '\r' || text[after] == '\n':
			after++
		case after+1 < len(text) && text[after] == '<' && text[after+1] == '/':
			return true
		default:
			return false
		}
	}
	return false
}

func cdataOffsetIsInsideMarkdownFence(fragment string) bool {
	if fragment == "" {
		return false
	}
	lines := strings.SplitAfter(fragment, "\n")
	inFence := false
	fenceMarker := ""
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if !inFence {
			if marker, ok := parseFenceOpen(trimmed); ok {
				inFence = true
				fenceMarker = marker
			}
			continue
		}
		if isFenceClose(trimmed, fenceMarker) {
			inFence = false
			fenceMarker = ""
		}
	}
	return inFence
}

func findXMLTagEnd(text string, from int) int {
	quote := rune(0)
	for i := maxInt(from, 0); i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		ch := normalizeFullwidthASCII(r)
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i += size
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			i += size
			continue
		}
		if ch == '>' {
			return i + size - 1
		}
		i += size
	}
	return -1
}

func hasXMLTagBoundary(text string, idx int) bool {
	if idx >= len(text) {
		return true
	}
	switch text[idx] {
	case ' ', '\t', '\n', '\r', '>', '/':
		return true
	default:
		r, _ := utf8.DecodeRuneInString(text[idx:])
		return normalizeFullwidthASCII(r) == '>'
	}
}

func isSelfClosingXMLTag(startTag string) bool {
	return strings.HasSuffix(strings.TrimSpace(startTag), "/")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parseXMLTagAttributes(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, m := range xmlAttrPattern.FindAllStringSubmatch(raw, -1) {
		if len(m) < 5 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		if key == "" {
			continue
		}
		value := m[3]
		if value == "" {
			value = m[4]
		}
		out[key] = value
	}
	return out
}

func parseStructuredCDATAParameterValue(paramName, raw string) (any, bool) {
	if preservesCDATAStringParameter(paramName) {
		return nil, false
	}
	normalized := normalizeCDATAForStructuredParse(raw)
	if !strings.Contains(normalized, "<") || !strings.Contains(normalized, ">") {
		return nil, false
	}
	if !cdataFragmentLooksExplicitlyStructured(normalized) {
		return nil, false
	}
	parsed, ok := parseXMLFragmentValue(normalized)
	if !ok {
		return nil, false
	}
	switch v := parsed.(type) {
	case []any:
		return v, true
	case map[string]any:
		if len(v) == 0 {
			return nil, false
		}
		return v, true
	default:
		return nil, false
	}
}

func normalizeCDATAForStructuredParse(raw string) string {
	if raw == "" {
		return ""
	}
	normalized := cdataBRSeparatorPattern.ReplaceAllString(raw, "\n")
	return html.UnescapeString(strings.TrimSpace(normalized))
}

// Preserve flat CDATA fragments as strings. Only recover structure when the
// fragment clearly encodes a data shape: multiple sibling elements, nested
// child elements, or an explicit item list.
func cdataFragmentLooksExplicitlyStructured(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}

	dec := xml.NewDecoder(strings.NewReader("<root>" + trimmed + "</root>"))
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	start, ok := tok.(xml.StartElement)
	if !ok || !strings.EqualFold(start.Name.Local, "root") {
		return false
	}

	depth := 0
	directChildren := 0
	firstChildName := ""
	firstChildHasNested := false

	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				directChildren++
				if directChildren == 1 {
					firstChildName = strings.ToLower(strings.TrimSpace(t.Name.Local))
				} else {
					return true
				}
			} else if directChildren == 1 && depth == 1 {
				firstChildHasNested = true
			}
			depth++
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "root") {
				if directChildren != 1 {
					return false
				}
				if firstChildName == "item" {
					return true
				}
				return firstChildHasNested
			}
			if depth > 0 {
				depth--
			}
		}
	}
}

func preservesCDATAStringParameter(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "content", "file_content", "text", "prompt", "query", "command", "cmd", "script", "code", "old_string", "new_string", "pattern", "path", "file_path":
		return true
	default:
		return false
	}
}
