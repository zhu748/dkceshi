package promptcompat

import (
	"errors"
	"strings"
)

// A repair response uses the same native payload validation as input adapters.
// Metadata may accompany text; a second executable/media payload may not.
func ValidateGeminiTextPart(part map[string]any) error {
	if _, ok := part["text"].(string); !ok {
		return errors.New("gemini text part requires string text")
	}
	for key := range part {
		switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
		case "functioncall", "functionresponse", "inlinedata", "filedata", "executablecode", "codeexecutionresult", "videometadata", "attachments", "files":
			return errors.New("gemini text part cannot contain a second payload")
		}
	}
	return validateKnownContentFields(part, true)
}
