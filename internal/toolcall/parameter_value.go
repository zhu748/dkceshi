package toolcall

import "strings"

func parseInvokeParameterValue(paramName, raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if value, ok := extractStandaloneCDATA(trimmed); ok {
		if preservesCDATAStringParameter(paramName) {
			return value
		}
		if parsed, ok := parseJSONLiteralValue(value); ok {
			if parsedArray, ok := coerceArrayValue(parsed, paramName); ok {
				return parsedArray
			}
			return parsed
		}
		if parsed, ok := parseStructuredCDATAParameterValue(paramName, value); ok {
			return parsed
		}
		if parsed, ok := parseLooseJSONArrayValue(value, paramName); ok {
			return parsed
		}
		return value
	}
	// Valid JSON is already unambiguous; XML/HTML recovery must not rewrite it.
	if parsed, ok := parseJSONLiteralValue(trimmed); ok {
		if array, ok := coerceArrayValue(parsed, paramName); ok {
			return array
		}
		return parsed
	}
	// Nested XML decodes its own leaf values. Pre-decoding the entire tree
	// would decode entities a second time when the child parser reads them.
	decoded := trimmed
	if !strings.Contains(decoded, "<") || !strings.Contains(decoded, ">") {
		decoded = extractRawTagValue(trimmed)
	}
	if parsed, ok := parseJSONLiteralValue(decoded); ok {
		if parsedArray, ok := coerceArrayValue(parsed, paramName); ok {
			return parsedArray
		}
		return parsed
	}
	if strings.Contains(decoded, "<") && strings.Contains(decoded, ">") {
		if parsedValue, ok := parseXMLFragmentValue(decoded); ok {
			switch v := parsedValue.(type) {
			case map[string]any:
				if len(v) > 0 {
					if parsedArray, ok := coerceArrayValue(v, paramName); ok {
						return parsedArray
					}
					return v
				}
			case []any:
				return v
			case string:
				text := strings.TrimSpace(v)
				if text == "" {
					return ""
				}
				if parsedText, ok := parseJSONLiteralValue(text); ok {
					if parsedArray, ok := coerceArrayValue(parsedText, paramName); ok {
						return parsedArray
					}
					return parsedText
				}
				if parsedText, ok := parseLooseJSONArrayValue(text, paramName); ok {
					return parsedText
				}
				return v
			default:
				return v
			}
		}
		if parsed := parseStructuredToolCallInput(decoded); len(parsed) > 0 {
			if len(parsed) == 1 {
				if rawValue, ok := parsed["_raw"].(string); ok {
					if parsedText, ok := parseLooseJSONArrayValue(rawValue, paramName); ok {
						return parsedText
					}
					return rawValue
				}
			}
			if parsedArray, ok := coerceArrayValue(parsed, paramName); ok {
				return parsedArray
			}
			return parsed
		}
	}
	if parsed, ok := parseLooseJSONArrayValue(decoded, paramName); ok {
		return parsed
	}
	return decoded
}
