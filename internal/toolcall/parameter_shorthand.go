package toolcall

import "strings"

// Some upstream calls omit `parameter name="` but retain the field name and
// closing quote, e.g. <|EPSE|description">...</|EPSE|parameter>. Recover only
// this exact shell; the ordinary parser still requires a balanced parameter.
// Values, missing fields and plain XML tags are never inferred here.
func scanFieldNamedParameter(text string, start, from int) (ToolMarkupTag, bool) {
	i := from
	for i < len(text) {
		ch := text[i]
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' ||
			i > from && (ch >= '0' && ch <= '9' || ch == '-' || ch == '.' || ch == ':') {
			i++
			continue
		}
		break
	}
	if i == from || i >= len(text) || text[i] != '"' && text[i] != '\'' {
		return ToolMarkupTag{}, false
	}
	name := text[from:i]
	switch strings.ToLower(name) {
	case "tool_calls", "tool-calls", "toolcalls", "invoke", "parameter":
		return ToolMarkupTag{}, false
	}
	end := i + 1
	for end < len(text) && strings.ContainsRune(" \t\r\n", rune(text[end])) {
		end++
	}
	if end >= len(text) || text[end] != '>' {
		return ToolMarkupTag{}, false
	}
	return ToolMarkupTag{
		Start: start, End: end, NameStart: from, NameEnd: i,
		Name: "parameter", EPSELike: true, ParameterName: name,
	}, true
}
