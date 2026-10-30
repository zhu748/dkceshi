package jsonvalue

import (
	"encoding/json"
	"testing"
)

func TestLosslessNumbersAtAllDepths(t *testing.T) {
	const input = `{"id":9007199254740993,"nested":[123456789012345678901234567890,0.1234567890123456789],"safe":1}`
	var got map[string]any
	if err := Unmarshal([]byte(input), &got); err != nil {
		t.Fatal(err)
	}
	if got["safe"] != float64(1) {
		t.Fatal("safe number representation changed")
	}
	out, _ := json.Marshal(got)
	var want map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &want); err != nil {
		t.Fatal(err)
	}
	if string(want["id"]) != "9007199254740993" || string(want["nested"]) != "[123456789012345678901234567890,0.1234567890123456789]" {
		t.Fatalf("precision lost: %s", out)
	}
}
func TestRejectsTrailingJSON(t *testing.T) {
	var v any
	if err := Unmarshal([]byte(`{} {}`), &v); err == nil {
		t.Fatal("accepted trailing value")
	}
}
