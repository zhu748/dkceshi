package promptcompat

import "fmt"

// Validate source metadata before adapters select roles or split content blocks.
// Missing/null optional fields retain their compatibility meaning.
func ValidateMessageMetadata(message map[string]any) error {
	if err := validateHistoryIdentityFields(message); err != nil {
		return err
	}
	if value := message["role"]; value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("role must be a string")
		}
	}
	for _, key := range []string{"prefix", "is_error"} {
		if value := message[key]; value != nil {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	return nil
}

// These fields identify a caller-owned message boundary, even for an empty
// assistant body. Native Responses call IDs remain on their individual calls.
func copyPromptMessageMetadata(out, source map[string]any) {
	for _, key := range []string{"name", "id", "prefix", "tool_call_id", "is_error"} {
		if value, present := source[key]; present {
			out[key] = value
		}
	}
}

func hasPromptMessageMetadata(message map[string]any) bool {
	for _, key := range []string{"name", "id", "prefix", "refusal", "audio", "tool_call_id", "is_error"} {
		if message[key] != nil {
			return true
		}
	}
	return false
}

func hasExplicitPromptMessage(message map[string]any) bool {
	_, content := message["content"]
	return content || hasPromptMessageMetadata(message)
}
