package gemini

import (
	"fmt"

	"ds2api/internal/promptcompat"
)

func applyGeminiOutputFormat(standard promptcompat.StandardRequest, req map[string]any) (promptcompat.StandardRequest, error) {
	cfg, _ := req["generationConfig"].(map[string]any)
	schema := cfg["responseJsonSchema"]
	if legacy := cfg["responseSchema"]; legacy != nil {
		if schema != nil {
			return standard, fmt.Errorf("provide only one of responseSchema and responseJsonSchema")
		}
		schema = normalizeGeminiSchema(legacy)
	}
	mime := asString(cfg["responseMimeType"])
	if mime != "" && mime != "text/plain" && mime != "application/json" {
		return standard, fmt.Errorf("unsupported responseMimeType %q", mime)
	}
	if schema != nil {
		if mime == "text/plain" {
			return standard, fmt.Errorf("response schema requires application/json")
		}
		return promptcompat.ApplyOutputFormat(standard, map[string]any{"type": "json_schema", "schema": schema})
	}
	if mime == "application/json" {
		return promptcompat.ApplyOutputFormat(standard, map[string]any{"type": "json"})
	}
	return standard, nil
}
