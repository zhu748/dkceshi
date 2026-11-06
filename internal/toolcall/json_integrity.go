package toolcall

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"strings"
)

func conflictingJSONKeys(raw string) bool {
	raw = strings.TrimSpace(raw)
	return json.Valid([]byte(raw)) && (jsonvalue.CheckDuplicateKeys([]byte(raw)) != nil || jsonvalue.CheckStringEncoding([]byte(raw)) != nil)
}
