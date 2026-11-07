package tooldiagnostics

import (
	"encoding/json"
	"testing"
)

func TestRepairExtraBodyRejectsAmbiguousJSONAndGenerationAliases(t *testing.T) {
	for _, raw := range []string{
		`{"temperature":0,"temperature":1}`,
		`{"temperature":0,"\u0074emperature":1}`,
		`{"stop":"\ud800"}`,
		`{"generationConfig":{"max_output_tokens":1,"maxOutputTokens":8192}}`,
		`{"generation_config":{"thinking_config":{"thinking_budget":1,"thinkingBudget":8192}}}`,
	} {
		if err := validateRepairExtraBody(json.RawMessage(raw)); err == nil {
			t.Fatalf("ambiguous repair configuration accepted: %s", raw)
		}
	}
	for _, raw := range []string{`{"generation_config":{"max_output_tokens":8192}}`, `{"stop":"\\ud800"}`, `{"metadata":{"maxOutputTokens":1,"max_output_tokens":2}}`} {
		if err := validateRepairExtraBody(json.RawMessage(raw)); err != nil {
			t.Fatalf("unambiguous config or opaque metadata rejected: %v", err)
		}
	}
}
