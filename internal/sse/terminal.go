package sse

import "strings"

// Full response snapshots can carry the final state without a separate patch.
func responseSnapshotFinished(path string, value any) bool {
	envelope, ok := value.(map[string]any)
	if !ok {
		return false
	}
	response, _ := envelope["response"].(map[string]any)
	if path == "response" || path == "" && response == nil {
		response = envelope
	}
	for _, key := range []string{"status", "quasi_status"} {
		status, _ := response[key].(string)
		if strings.EqualFold(strings.TrimSpace(status), "FINISHED") {
			return true
		}
	}
	return false
}
