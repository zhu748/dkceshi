package toolcall

import (
	"encoding/json"
	"testing"
)

func TestRegressionLargeIntegerToolArgument(t *testing.T) {
	calls := ParseToolCalls(`<tool_calls><invoke name="lookup"><parameter name="id">9007199254740993</parameter></invoke></tool_calls>`, []string{"lookup"})
	if len(calls) != 1 {
		t.Fatalf("missing call: %#v", calls)
	}
	b, err := json.Marshal(calls[0].Input)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"id":9007199254740993}` {
		t.Fatalf("numeric tool argument changed: %s", b)
	}
}
