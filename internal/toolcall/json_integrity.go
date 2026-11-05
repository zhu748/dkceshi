package toolcall

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"strings"
)

func conflictingJSONKeys(raw string) bool {
	raw = strings.TrimSpace(raw)
	return len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') && json.Valid([]byte(raw)) && jsonvalue.CheckDuplicateKeys([]byte(raw)) != nil
}
