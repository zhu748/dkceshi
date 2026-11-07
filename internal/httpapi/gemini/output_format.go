package gemini

import (
	"fmt"

	"ds2api/internal/promptcompat"
)

func applyGeminiOutputFormat(standard promptcompat.StandardRequest, req map[string]any) (promptcompat.StandardRequest, error) {
	var selected any
	seen := false
	for _, key := range []string{"generationConfig", "generation_config"} {
		if req[key] == nil {
			continue
		}
		cfg, ok := req[key].(map[string]any)
		if !ok {
			return standard, fmt.Errorf("%s must be an object", key)
		}
		format, err := geminiOutputFormat(cfg)
		if err != nil {
			return standard, err
		}
		if seen {
			if _, _, err := promptcompat.ResolvePayloadAliases(map[string]any{"generationConfig": selected, "generation_config": format}, "generationConfig", "generation_config"); err != nil {
				return standard, err
			}
		}
		selected, seen = format, true
	}
	return promptcompat.ApplyOutputFormat(standard, selected)
}

func geminiOutputFormat(cfg map[string]any) (any, error) {
	schema, _, err := promptcompat.ResolvePayloadAliases(cfg, "responseJsonSchema", "response_json_schema")
	if err != nil {
		return nil, err
	}
	legacy, _, err := promptcompat.ResolvePayloadAliases(cfg, "responseSchema", "response_schema")
	if err != nil {
		return nil, err
	}
	if legacy != nil {
		if schema != nil {
			return nil, fmt.Errorf("provide only one of responseSchema and responseJsonSchema")
		}
		schema = normalizeGeminiSchema(legacy)
	}
	rawMime, _, err := promptcompat.ResolvePayloadAliases(cfg, "responseMimeType", "response_mime_type")
	if err != nil {
		return nil, err
	}
	mime := ""
	if rawMime != nil {
		var ok bool
		mime, ok = rawMime.(string)
		if !ok {
			return nil, fmt.Errorf("responseMimeType must be a string")
		}
	}
	if mime != "" && mime != "text/plain" && mime != "application/json" {
		return nil, fmt.Errorf("unsupported responseMimeType %q", mime)
	}
	if schema != nil {
		if mime == "text/plain" {
			return nil, fmt.Errorf("response schema requires application/json")
		}
		return map[string]any{"type": "json_schema", "schema": schema}, nil
	}
	if mime == "application/json" {
		return map[string]any{"type": "json"}, nil
	}
	return nil, nil
}
