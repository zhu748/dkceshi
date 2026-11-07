package promptcompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Native fields must be checked before adapters choose aliases or supply
// defaults. Once flattened, conflicting IDs and overwritten values are lost.
func validateNativeToolContext(block map[string]any, nativePart bool) error {
	if !nativePart {
		switch strings.ToLower(strings.TrimSpace(asString(block["type"]))) {
		case "tool_use":
			if err := validateHistoryIdentityFields(block); err != nil {
				return err
			}
			if err := validateIDFields(block, "id", "tool_use_id", "tool_call_id"); err != nil {
				return err
			}
			return validateNativeArgumentObject(block, "input")
		case "tool_result":
			if err := validateHistoryIdentityFields(block); err != nil {
				return err
			}
			if err := validateIDFields(block, "tool_use_id", "tool_call_id"); err != nil {
				return err
			}
			if value, present := block["is_error"]; present {
				if _, ok := value.(bool); !ok {
					return fmt.Errorf("is_error must be a boolean")
				}
			}
			return validatePromptMessageContent(block["content"])
		}
		return nil
	}
	for _, key := range []string{"functionCall", "functionResponse"} {
		function, ok := block[key].(map[string]any)
		if !ok {
			continue // Container shape is checked by ValidateContentContainers.
		}
		if err := validateHistoryIdentityFields(function); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		if err := validateIDFields(function, "id", "call_id", "callId", "tool_call_id"); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		if key == "functionCall" {
			if err := validateNativeArgumentObject(function, "args"); err != nil {
				return err
			}
		} else {
			if err := validatePayloadAliases(function, "response", "output"); err != nil {
				return err
			}
			if _, present := function["parts"]; present {
				if err := ValidateContentContainers([]any{function}, "parts", false); err != nil {
					return fmt.Errorf("functionResponse: %w", err)
				}
			}
		}
	}
	return nil
}

func validateNativeArgumentObject(block map[string]any, field string) error {
	if value, present := block[field]; present {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be an argument object", field)
		}
	}
	return nil
}

func validateIDFields(block map[string]any, fields ...string) error {
	var first any
	for _, field := range fields {
		value := block[field]
		if err := validateStringAliases(first, value, "tool call ID"); err != nil {
			return err
		}
		if text := strings.TrimSpace(asString(value)); text != "" {
			first = value
		}
	}
	return nil
}

// Result payloads are JSON data: a string containing JSON remains a string,
// and an explicit null must not fall through to a different output value.
func validatePayloadAliases(block map[string]any, fields ...string) error {
	presentFields := 0
	for _, field := range fields {
		if _, present := block[field]; present {
			presentFields++
		}
	}
	if presentFields < 2 {
		return nil // Normal single-payload histories need no extra JSON copy.
	}
	var first []byte
	for _, field := range fields {
		value, present := block[field]
		if !present {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%s cannot be encoded: %w", field, err)
		}
		if first != nil && string(first) != string(data) {
			return fmt.Errorf("conflicting payload aliases: %s", field)
		}
		first = data
	}
	return nil
}

// ResolvePayloadAliases compares every supplied JSON spelling before selecting
// one. Presence is separate from null so adapters cannot overwrite explicit data.
func ResolvePayloadAliases(block map[string]any, fields ...string) (any, bool, error) {
	if err := validatePayloadAliases(block, fields...); err != nil {
		return nil, false, err
	}
	for _, field := range fields {
		if value, present := block[field]; present {
			return value, true, nil
		}
	}
	return nil, false, nil
}
