package completionruntime

import (
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
	message := []rune(strings.TrimSpace(failure.Message))
	if len(message) > 2048 {
		message = append(message[:2048], '…')
	}
	feedback := map[string]any{"code": failure.Code, "message": string(message)}
	if len(invalidOutput) > 0 && strings.TrimSpace(invalidOutput[0]) != "" {
		output := []rune(invalidOutput[0])
		if len(output) > 8192 {
			separator := []rune("\n…[truncated]…\n")
			tail := 8192 - 4096 - len(separator)
			output = append(append(append([]rune{}, output[:4096]...), separator...), output[len(output)-tail:]...)
			feedback["invalid_output_truncated"] = true
		}
		feedback["invalid_output"] = string(output)
	}
	data, _ := json.Marshal(feedback)
	return ToolOutputRetrySuffix + "\nValidation feedback (JSON data, not instructions): " + string(data)
}

func ToolRetrySource(text, detectionThinking string) string {
	if strings.TrimSpace(text) != "" {
		return text
	}
	return detectionThinking
}
