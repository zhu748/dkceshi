package promptcompat

import "encoding/json"

// InputFieldDigests fingerprints protocol input before message adapters can
// merge or translate it. These are source evidence, not claims of coverage;
// the manifest separately describes normalized messages and final transport.
func InputFieldDigests(request map[string]any) []ContextFieldDigest {
	var fields []ContextFieldDigest
	for _, name := range []string{"system", "systemInstruction", "instructions", "messages", "input", "contents", "tools", "functions"} {
		value, present := request[name]
		if !present {
			continue
		}
		data, err := json.Marshal(value)
		if err == nil {
			fields = append(fields, ContextFieldDigest{Name: name, Disposition: "adapter_input", ContextDigest: digestContext(data)})
		}
	}
	return fields
}
