package gemini

import "ds2api/internal/promptcompat"

// Resolve before adaptation, without overwriting the caller's source evidence.
func normalizeGeminiSystemAliases(req map[string]any) (map[string]any, error) {
	if _, present := req["system_instruction"]; !present {
		return req, nil
	}
	system, _, err := promptcompat.ResolvePayloadAliases(req, "systemInstruction", "system_instruction")
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(req)+1)
	for key, value := range req {
		out[key] = value
	}
	out["systemInstruction"] = system
	return out, nil
}
