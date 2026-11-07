package promptcompat

import (
	"fmt"
	"strings"
)

func toolSelectorName(selector map[string]any) (string, error) {
	if value, present := selector["type"]; present {
		if _, ok := value.(string); !ok {
			return "", fmt.Errorf("tool selector type must be a string")
		}
	}
	function := map[string]any(nil)
	if raw, present := selector["function"]; present {
		var ok bool
		function, ok = raw.(map[string]any)
		if !ok {
			return "", fmt.Errorf("tool selector function must be an object")
		}
	}
	for _, object := range []map[string]any{selector, function} {
		if raw, present := object["name"]; present {
			if _, ok := raw.(string); !ok {
				return "", fmt.Errorf("tool selector name must be a string")
			}
		}
	}
	if err := validateStringAliases(selector["name"], function["name"], "tool selector name"); err != nil {
		return "", err
	}
	if name := strings.TrimSpace(asString(selector["name"])); name != "" {
		return name, nil
	}
	return strings.TrimSpace(asString(function["name"])), nil
}
