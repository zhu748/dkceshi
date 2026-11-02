package toolcall

import "strings"

// An array's final ] can be mistaken for part of the CDATA ]]> delimiter.
// Recover only one missing outer array close after a complete compound item.
// The caller must validate the entire candidate against the declared schema;
// never close inner objects/strings, remove trailing text or invent values.
func parseArrayMissingClose(raw string) (any, bool) {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "[") || (!strings.HasSuffix(text, "}") && !strings.HasSuffix(text, "]")) {
		return nil, false
	}
	return parseJSONLiteralValue(text + "]")
}
