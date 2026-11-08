package promptcompat

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

var inputPathEscaper = strings.NewReplacer("~", "~0", "/", "~1")

const maxInputSourceFields = 4096
const maxInputSourcePathLength = 4096
const maxInputSourcePathBytes = 256 << 10
const maxInputSourceEncodedBytes = 4 << 20

// Capture per-field evidence before adapters merge native messages and blocks.
// Traverse only protocol containers, never arbitrary tool arguments or business
// JSON. Paths/hashes contain no source values and do not claim full coverage.
func appendInputItemDigests(out []ContextFieldDigest, raw any, path string) []ContextFieldDigest {
	type entry struct {
		value any
		path  string
		index int
	}
	pending := []entry{{value: raw, path: path}}
	count := 0
	pathBytes := 0
	encodedBytes := 0
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		switch value := current.value.(type) {
		case []any:
			if i := current.index; i < len(value) {
				pending = append(pending, entry{value: value, path: current.path, index: i + 1}, entry{value: value[i], path: current.path + "/" + strconv.Itoa(i)})
			}
		case map[string]any:
			keys := make([]string, 0, len(value))
			for key := range value {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fieldPath := current.path + "/" + inputPathEscaper.Replace(key)
				if count >= maxInputSourceFields || len(fieldPath) > maxInputSourcePathLength || pathBytes+len(key)+len(fieldPath) > maxInputSourcePathBytes {
					return append(out, ContextFieldDigest{Name: "source_field_trace", Path: path, Disposition: "adapter_source_trace_limit"})
				}
				data, err := json.Marshal(value[key])
				if err != nil {
					continue
				}
				if encodedBytes+len(data) > maxInputSourceEncodedBytes {
					return append(out, ContextFieldDigest{Name: "source_field_trace", Path: path, Disposition: "adapter_source_trace_limit"})
				}
				out = append(out, ContextFieldDigest{Name: key, Path: fieldPath, Disposition: "adapter_source_field", ContextDigest: digestContext(data)})
				count++
				pathBytes += len(key) + len(fieldPath)
				encodedBytes += len(data)
			}
			for i := len(keys) - 1; i >= 0; i-- {
				key := keys[i]
				switch key {
				case "content", "parts", "summary", "tool_calls":
					if children, ok := value[key].([]any); ok {
						pending = append(pending, entry{value: children, path: current.path + "/" + key})
					}
				case "functionCall", "functionResponse", "function", "function_call":
					if child, ok := value[key].(map[string]any); ok {
						pending = append(pending, entry{value: child, path: current.path + "/" + key})
					}
				}
			}
		}
	}
	return out
}
