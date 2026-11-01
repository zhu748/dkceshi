package toolcall

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"strings"
)

func parseToolCallInput(v any) map[string]any {
	switch x := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return x
	case string:
		raw := strings.TrimSpace(x)
		if raw == "" {
			return map[string]any{}
		}
		var parsed map[string]any
		if err := jsonvalue.Unmarshal([]byte(raw), &parsed); err == nil && parsed != nil {
			return parsed
		}
		// Try to repair invalid backslashes (common in Windows paths output by models)
		repaired := repairInvalidJSONBackslashes(raw)
		if repaired != raw {
			if err := jsonvalue.Unmarshal([]byte(repaired), &parsed); err == nil && parsed != nil {
				return parsed
			}
		}
		// Try to repair loose JSON in string argument as well
		repairedLoose := RepairLooseJSON(raw)
		if repairedLoose != raw {
			if err := jsonvalue.Unmarshal([]byte(repairedLoose), &parsed); err == nil && parsed != nil {
				return parsed
			}
		}
		return map[string]any{"_raw": raw}
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return map[string]any{}
		}
		var parsed map[string]any
		if err := jsonvalue.Unmarshal(b, &parsed); err == nil && parsed != nil {
			return parsed
		}
		return map[string]any{}
	}
}
