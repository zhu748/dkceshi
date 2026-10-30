package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"ds2api/internal/toolcall"
)

var promptXMLTextEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

const (
	promptEPSEToolCallsOpen  = "<|EPSE|tool_calls>"
	promptEPSEToolCallsClose = "</|EPSE|tool_calls>"
	promptEPSEInvokeOpen     = "<|EPSE|invoke"
	promptEPSEInvokeClose    = "</|EPSE|invoke>"
	promptEPSEParameterOpen  = "<|EPSE|parameter"
	promptEPSEParameterClose = "</|EPSE|parameter>"
)

// FormatToolCallsForPrompt renders a tool_calls slice into the prompt-visible
// invoke/parameter history block used across adapters.
func FormatToolCallsForPrompt(raw any) string {
	return FormatToolCallsForPromptForMarker(raw, "EPSE")
}

// FormatToolCallsForPromptForMarker is the marker-aware variant of
// FormatToolCallsForPrompt. It renders the block with the caller-specific
// tool-call marker so prompt history stays consistent with the format spec.
func FormatToolCallsForPromptForMarker(raw any, marker string) string {
	out := formatToolCallsForPromptEPSE(raw)
	marker = strings.TrimSpace(marker)
	if marker == "" || marker == "EPSE" {
		return out
	}
	return toolcall.ApplyToolMarker(out, marker)
}

func formatToolCallsForPromptEPSE(raw any) string {
	calls, ok := raw.([]any)
	if !ok || len(calls) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(calls))
	for _, item := range calls {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := formatToolCallForPrompt(call)
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return promptEPSEToolCallsOpen + "\n" + strings.Join(blocks, "\n") + "\n" + promptEPSEToolCallsClose
}

// StringifyToolCallArguments normalizes tool arguments into a compact string
// while preserving raw concatenated payloads when they already look like model
// output rather than a single JSON object.
func StringifyToolCallArguments(v any) string {
	switch x := v.(type) {
	case nil:
		return "{}"
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "{}"
		}
		s = normalizeToolArgumentString(s)
		if s == "" {
			return "{}"
		}
		return s
	default:
		b, err := json.Marshal(x)
		if err != nil || len(b) == 0 {
			return "{}"
		}
		return string(b)
	}
}

func formatToolCallForPrompt(call map[string]any) string {
	if call == nil {
		return ""
	}

	name := strings.TrimSpace(asString(call["name"]))
	fn, _ := call["function"].(map[string]any)
	if name == "" && fn != nil {
		name = strings.TrimSpace(asString(fn["name"]))
	}
	if name == "" {
		return ""
	}

	argsRaw := call["arguments"]
	if argsRaw == nil {
		argsRaw = call["input"]
	}
	if argsRaw == nil && fn != nil {
		argsRaw = fn["arguments"]
		if argsRaw == nil {
			argsRaw = fn["input"]
		}
	}

	nameAttr := escapeXMLAttribute(name)
	if id := strings.TrimSpace(asString(call["id"])); id != "" {
		nameAttr += `" call_id="` + escapeXMLAttribute(id)
	}
	parameters := formatToolCallParametersForPrompt(argsRaw)
	if parameters == "" {
		return `  ` + promptEPSEInvokeOpen + ` name="` + nameAttr + `">` + promptEPSEInvokeClose
	}

	return "  " + promptEPSEInvokeOpen + " name=\"" + nameAttr + "\">\n" +
		parameters + "\n" +
		"  " + promptEPSEInvokeClose
}

func formatToolCallParametersForPrompt(raw any) string {
	value := normalizePromptToolCallValue(raw)
	body, ok := renderPromptToolParameters(value, "    ")
	if ok {
		return body
	}

	fallback := StringifyToolCallArguments(raw)
	if strings.TrimSpace(fallback) == "" {
		return ""
	}
	return "    " + promptEPSEParameterOpen + " name=\"content\">" + renderPromptXMLText(fallback) + promptEPSEParameterClose
}

func renderPromptToolParameters(value any, indent string) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", true
	case map[string]any:
		if len(v) == 0 {
			return "", true
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			rendered, ok := renderPromptParameterNode(key, v[key], indent)
			if !ok {
				return "", false
			}
			lines = append(lines, rendered)
		}
		return strings.Join(lines, "\n"), true
	case []any:
		lines := make([]string, 0, len(v))
		for _, item := range v {
			rendered, ok := renderPromptParameterNode("item", item, indent)
			if !ok {
				return "", false
			}
			lines = append(lines, rendered)
		}
		return strings.Join(lines, "\n"), true
	case string:
		return indent + promptEPSEParameterOpen + ` name="content">` + renderPromptXMLText(v) + promptEPSEParameterClose, true
	default:
		return indent + promptEPSEParameterOpen + ` name="value">` + renderPromptXMLText(fmt.Sprint(v)) + promptEPSEParameterClose, true
	}
}

// JSON values inside parameters preserve empty collections, nested array shape,
// numeric precision and keys that cannot be represented as XML element names.
func renderPromptParameterNode(name string, value any, indent string) (string, bool) {
	if name == "" {
		return "", false
	}
	var body string
	if text, ok := value.(string); ok {
		body = renderPromptXMLText(text)
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", false
		}
		body = escapeXMLText(string(encoded))
	}
	return indent + promptEPSEParameterOpen + ` name="` + escapeXMLAttribute(name) + `">` + body + promptEPSEParameterClose, true
}

func normalizePromptToolCallValue(raw any) any {
	switch x := raw.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return ""
		}
		var parsed any
		decoder := json.NewDecoder(bytes.NewBufferString(s))
		decoder.UseNumber()
		if err := decoder.Decode(&parsed); err == nil {
			var trailing any
			if decoder.Decode(&trailing) == io.EOF {
				return parsed
			}
		}
		return x
	default:
		return x
	}
}

// renderPromptXMLText emits CDATA for every string so prompt-visible tool
// history stays uniform and does not drift back toward ad-hoc escaping.
func renderPromptXMLText(text string) string {
	if strings.Contains(text, "]]>") {
		return "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
	}
	return "<![CDATA[" + text + "]]>"
}

func escapeXMLAttribute(text string) string {
	if text == "" {
		return ""
	}
	return strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(text)
}

func normalizeToolArgumentString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if looksLikeConcatenatedJSON(trimmed) {
		// Keep the original payload to avoid silently rewriting model output.
		return raw
	}
	return trimmed
}

func looksLikeConcatenatedJSON(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "}{") || strings.Contains(trimmed, "][") {
		return true
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	var first any
	if err := dec.Decode(&first); err != nil {
		return false
	}
	var second any
	return dec.Decode(&second) == nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func escapeXMLText(v string) string {
	if v == "" {
		return ""
	}
	return promptXMLTextEscaper.Replace(v)
}
