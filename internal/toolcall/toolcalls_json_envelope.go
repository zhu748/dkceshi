package toolcall

// A compatibility envelope may contain one object-valued input/parameters key,
// or be empty for an argument-free call. Never choose one of several argument
// containers or silently drop sibling fields: they still carry model intent.
func parseJSONToolCallEnvelope(name string, payload map[string]any) (ParsedToolCall, bool) {
	if len(payload) == 0 {
		return ParsedToolCall{Name: name, Input: map[string]any{}}, true
	}
	if len(payload) != 1 {
		return ParsedToolCall{}, false
	}
	value, present := payload["input"]
	if !present {
		value = payload["parameters"]
	}
	input, ok := value.(map[string]any)
	if !ok {
		return ParsedToolCall{}, false
	}
	return ParsedToolCall{Name: name, Input: input}, true
}
