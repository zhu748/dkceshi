package promptcompat

import "encoding/json"

const inputContextSnapshotKey = "_input_context_snapshot"

type inputContextSnapshot []ContextFieldDigest

// Capture before caller flags or attachment preprocessing rewrite the source.
// The private type prevents client JSON from supplying trusted fingerprints.
func CaptureInputFieldDigests(request map[string]any) {
	if request == nil {
		return
	}
	if _, captured := request[inputContextSnapshotKey].(inputContextSnapshot); !captured {
		request[inputContextSnapshotKey] = inputContextSnapshot(InputFieldDigests(request))
	}
}

// InputFieldDigests fingerprints protocol input before message adapters can
// merge or translate it. These are source evidence, not claims of coverage;
// the manifest separately describes normalized messages and final transport.
func InputFieldDigests(request map[string]any) []ContextFieldDigest {
	if captured, ok := request[inputContextSnapshotKey].(inputContextSnapshot); ok {
		return append([]ContextFieldDigest(nil), captured...)
	}
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
