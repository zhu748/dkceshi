package assistantturn

import (
	"ds2api/internal/streambudget"
	"errors"
)

func readOutputError(err error) *OutputError {
	if errors.Is(err, streambudget.ErrLimit) {
		return &OutputError{Status: 502, Code: "output_limit_exceeded", Message: "Upstream output exceeded configured resource limits."}
	}
	return &OutputError{Status: 502, Code: "upstream_read_error", Message: "Upstream stream read failed: " + err.Error()}
}
