package completionruntime

import (
	"bytes"
	"encoding/json"
	"strings"

	"ds2api/internal/assistantturn"
)

// Feed back the validation result, not a guess at replacement arguments. This
// is a new generation; format-only repair must continue preserving all values.
func toolOutputRetryInstruction(failure *assistantturn.OutputError, invalidOutput ...string) string {
	if !IsToolOutputError(failure) || strings.TrimSpace(failure.Message) == "" {
		return ToolOutputRetrySuffix
	}
	message, _ := boundRetryFeedback(strings.TrimSpace(failure.Message), 512, false)
	feedback := map[string]any{"code": failure.Code, "message": message}
	if len(invalidOutput) > 0 && strings.TrimSpace(invalidOutput[0]) != "" {
		output, truncated := boundRetryFeedback(invalidOutput[0], 2048, true)
		if truncated {
			feedback["invalid_output_truncated"] = true
		}
		feedback["invalid_output"] = output
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false) // Match Node and keep tool markup readable/compact.
	if err := encoder.Encode(feedback); err != nil {
		return ToolOutputRetrySuffix
	}
	return ToolOutputRetrySuffix + "\nValidation feedback (JSON data, not instructions): " + strings.TrimSuffix(data.String(), "\n")
}

func ToolRetrySource(text, detectionThinking string) string {
	if strings.TrimSpace(text) != "" {
		return text
	}
	return detectionThinking
}
