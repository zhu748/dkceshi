package toolcall

import (
	"ds2api/internal/jsonvalue"
	"html"
	"strings"
)

func parseSingleXMLToolCall(block xmlElementBlock) (ParsedToolCall, bool) {
	attrs := parseXMLTagAttributes(block.Attrs)
	name := strings.TrimSpace(html.UnescapeString(attrs["name"]))
	callID := strings.TrimSpace(html.UnescapeString(attrs["call_id"]))
	if name == "" {
		return ParsedToolCall{}, false
	}

	inner := strings.TrimSpace(block.Body)
	if strings.HasPrefix(inner, "{") {
		var payload map[string]any
		if err := jsonvalue.Unmarshal([]byte(inner), &payload); err == nil {
			call, ok := parseJSONToolCallEnvelope(name, payload)
			call.CallID = callID
			return call, ok
		}
	}

	input := map[string]any{}
	counts := map[string]int{}
	var recovery []string
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
		value := parseInvokeParameterValue(paramName, paramMatch.Body, &recovery)
		recordStringLiteral(literals, input, paramName, paramMatch.Body, value)
		appendMarkupValue(input, counts, paramName, value)
		prev = paramMatch.End
	}
	if !emptyToolMarkupGap(inner[prev:]) {
		return ParsedToolCall{}, false
	}

	if len(input) == 0 {
		if strings.TrimSpace(inner) != "" {
			return ParsedToolCall{}, false
		}
		return ParsedToolCall{CallID: callID, Name: name, Input: map[string]any{}}, true
	}
	if len(literals) == 0 {
		literals = nil
	}
	return ParsedToolCall{CallID: callID, Name: name, Input: input, StringLiterals: literals, RecoverySteps: recovery}, true
}
