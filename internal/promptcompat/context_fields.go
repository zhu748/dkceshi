package promptcompat

import (
	"encoding/json"
	"sort"
	"strings"
)

// Coverage describes which serializer receives a standard message field. It
// does not claim every nested value survives sanitization or protocol adapters.
type ContextFieldDigest struct {
	Name        string `json:"name"`
	Disposition string `json:"disposition"`
	ContextDigest
}

func contextFieldDigests(message map[string]any, rendered bool) []ContextFieldDigest {
	keys := make([]string, 0, len(message))
	for key := range message {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]ContextFieldDigest, 0, len(keys))
	role := strings.ToLower(strings.TrimSpace(asString(message["role"])))
	for _, key := range keys {
		data, err := json.Marshal(message[key])
		if err != nil {
			continue
		}
		disposition := "unrendered"
		switch key {
		case "role", "name", "id", "tool_call_id", "is_error", "prefix":
			disposition = "message_metadata"
		case "content":
			disposition = "content_serializer"
		case "reasoning_content", "tool_calls", "function_call", "refusal", "audio":
			if role == "assistant" {
				disposition = "content_serializer"
			}
		case "cache_control":
			disposition = "transport_only"
		}
		if !rendered {
			disposition = "omitted_message"
		}
		fields = append(fields, ContextFieldDigest{Name: key, Disposition: disposition, ContextDigest: digestContext(data)})
	}
	return fields
}

func cloneContextDigests(source []ContextMessageDigest) []ContextMessageDigest {
	if source == nil {
		return nil
	}
	out := append([]ContextMessageDigest{}, source...)
	for i := range out {
		out[i].Fields = append([]ContextFieldDigest(nil), source[i].Fields...)
	}
	return out
}
