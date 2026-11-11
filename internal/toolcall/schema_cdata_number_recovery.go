package toolcall

import (
	"encoding/json"
	"strings"
)

// A duplicated number around a malformed CDATA opener has one recoverable
// value only when both complete JSON number tokens agree byte for byte.
// The normalization caller must reject string-capable schemas and validate
// the candidate against all numeric constraints before accepting it.
func parseRepeatedCDATANumber(raw string) (any, bool) {
	left, right, found := strings.Cut(strings.TrimSpace(raw), "</![CDATA[")
	if !found {
		return nil, false
	}
	number := strings.TrimSpace(left)
	if number == "" || number != strings.TrimSpace(right) {
		return nil, false
	}
	parsed, ok := parseJSONLiteralValue(number)
	if !ok {
		return nil, false
	}
	switch parsed.(type) {
	case float64, json.Number:
		return parsed, true
	default:
		return nil, false
	}
}
