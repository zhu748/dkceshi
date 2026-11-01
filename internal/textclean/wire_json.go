package textclean

import (
	"encoding/json"
	"io"
	"strings"
)

// Decode a complete value instead of deleting a regex-shaped substring. This
// handles key ordering and nested strings without crossing JSON boundaries.
func jsonValueEnd(text string) (int, bool) {
	dec := json.NewDecoder(strings.NewReader(text))
	var value json.RawMessage
	err := dec.Decode(&value)
	if err == nil {
		return int(dec.InputOffset()), false
	}
	return 0, err == io.ErrUnexpectedEOF || err == io.EOF
}

func isWireToolArray(text string) bool {
	var calls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal([]byte(text), &calls) != nil || len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		if !strings.HasPrefix(call.ID, "call") || call.Type != "function" || call.Function.Name == "" || len(call.Function.Arguments) == 0 {
			return false
		}
	}
	return true
}
