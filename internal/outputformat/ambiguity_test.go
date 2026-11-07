package outputformat

import "testing"

func TestStructuredOutputRejectsAmbiguousOrInvalidUnicodeJSON(t *testing.T) {
	for _, raw := range []string{`{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `"\ud800"`, `{"\udc00":1}`, "{\"x\":\"\xff\"}"} {
		if err := (Format{Type: "json"}).Validate(raw); err == nil {
			t.Errorf("ambiguous output accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{"id":9007199254740993,"label":"\ud83d\ude00"}`, `"\\ud800"`, `{"payload":"{\"x\":1,\"x\":2}"}`} {
		if err := (Format{Type: "json"}).Validate(raw); err != nil {
			t.Errorf("valid literal/number rejected: %s %v", raw, err)
		}
	}
}
