package toolcall

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"strings"

	"ds2api/internal/jsonvalue"
)

// RecoverDSMLToolCalls handles the complete native DSML grammar at the final
// repair boundary. The ordinary parser and stream sieve keep their existing
// semantics. Incomplete/ambiguous markup still needs the existing repair path.
// Native string=true values are copied verbatim, never reparsed as JSON/XML.
func RecoverDSMLToolCalls(text string) ([]ParsedToolCall, bool) {
	var calls []ParsedToolCall
	var recovered strings.Builder
	prev := 0
	for pos := 0; pos < len(text); {
		open, ok := FindDSMLTagOutsideIgnored(text, pos)
		if !ok {
			break
		}
		if open.Closing || open.Name != "calls" {
			return nil, false
		}
		attrs, ok := dsmlRecoveryAttrs(text, open)
		if !ok || len(attrs) != 0 {
			return nil, false
		}
		closeTag, ok := FindMatchingDSMLClose(text, open)
		if !ok {
			return nil, false
		}
		if closeAttrs, valid := dsmlRecoveryAttrs(text, closeTag); !valid || len(closeAttrs) != 0 {
			return nil, false
		}
		body := strings.TrimSpace(text[open.End+1 : closeTag.Start])
		if body == "" {
			return nil, false
		}
		recovered.WriteString(text[prev:open.Start])
		recovered.WriteString("<tool_calls>")
		for body != "" {
			invoke, inner, rest, ok := takeDSMLRecoveryElement(body, "invoke")
			if !ok {
				return nil, false
			}
			call, ok := recoverDSMLInvoke(invoke, inner)
			if !ok {
				return nil, false
			}
			calls = append(calls, call)
			payload, err := json.Marshal(map[string]any{"input": call.Input})
			if err != nil {
				return nil, false
			}
			recovered.WriteString(`<invoke name="` + html.EscapeString(call.Name) + `">`)
			recovered.Write(payload)
			recovered.WriteString("</invoke>")
			body = strings.TrimSpace(rest)
		}
		pos = closeTag.End + 1
		prev = pos
		recovered.WriteString("</tool_calls>")
	}
	if len(calls) == 0 {
		return nil, false
	}
	// Parse the entire batch after replacing only complete DSML spans. This
	// preserves interleaved canonical calls and their order and catches broken
	// canonical siblings that the DSML-only intent probe cannot recognize.
	recovered.WriteString(text[prev:])
	parsed := ParseStandaloneToolCallsDetailed(recovered.String(), nil)
	if parsed.SawToolCallIntent || len(parsed.Calls) < len(calls) {
		return nil, false
	}
	return parsed.Calls, true
}

func recoverDSMLInvoke(attrs map[string]string, body string) (ParsedToolCall, bool) {
	name := strings.TrimSpace(attrs["name"])
	if len(attrs) != 1 || name == "" {
		return ParsedToolCall{}, false
	}
	call := ParsedToolCall{Name: name, Input: map[string]any{}}
	for body = strings.TrimSpace(body); body != ""; {
		param, raw, rest, ok := takeDSMLRecoveryElement(body, "parameter")
		if !ok || len(param) != 2 || strings.TrimSpace(param["name"]) == "" {
			return ParsedToolCall{}, false
		}
		key := strings.TrimSpace(param["name"])
		if _, duplicate := call.Input[key]; duplicate {
			return ParsedToolCall{}, false
		}
		switch param["string"] {
		case "true":
			// The native flag declares a raw string. Even literal CDATA and
			// XML entities belong to its value; do not unwrap or unescape them.
			if _, nested := FindDSMLTagOutsideIgnored(raw, 0); nested {
				return ParsedToolCall{}, false
			}
			call.Input[key] = raw
		case "false":
			var value any
			if err := jsonvalue.Unmarshal([]byte(raw), &value); err != nil {
				return ParsedToolCall{}, false
			}
			call.Input[key] = value
		default:
			return ParsedToolCall{}, false
		}
		body = strings.TrimSpace(rest)
	}
	return call, true
}

func takeDSMLRecoveryElement(text, name string) (attrs map[string]string, body, rest string, ok bool) {
	open, ok := scanDSMLTagAt(text, 0)
	if !ok || open.Start != 0 || open.Closing || open.Name != name {
		return nil, "", "", false
	}
	attrs, ok = dsmlRecoveryAttrs(text, open)
	if !ok {
		return nil, "", "", false
	}
	closeTag, ok := FindMatchingDSMLClose(text, open)
	if !ok {
		return nil, "", "", false
	}
	if closeAttrs, valid := dsmlRecoveryAttrs(text, closeTag); !valid || len(closeAttrs) != 0 {
		return nil, "", "", false
	}
	return attrs, text[open.End+1 : closeTag.Start], text[closeTag.End+1:], true
}

func dsmlRecoveryAttrs(text string, tag DSMLTag) (map[string]string, bool) {
	raw := text[tag.Start : tag.End+1]
	// Local names in this conservative grammar are literal ASCII. The DSML
	// prefix itself has already been checked by the shared structural scanner.
	idx := strings.Index(strings.ToLower(raw), tag.Name)
	if idx < 0 || strings.HasSuffix(raw, "/>") {
		return nil, false
	}
	dec := xml.NewDecoder(strings.NewReader("<x" + raw[idx+len(tag.Name):len(raw)-1] + "/>"))
	token, err := dec.Token()
	if err != nil {
		return nil, false
	}
	start, ok := token.(xml.StartElement)
	if !ok {
		return nil, false
	}
	attrs := map[string]string{}
	for _, attr := range start.Attr {
		if attr.Name.Space != "" {
			return nil, false
		}
		if _, duplicate := attrs[attr.Name.Local]; duplicate {
			return nil, false
		}
		attrs[attr.Name.Local] = attr.Value
	}
	return attrs, true
}
