package toolcall

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func recursiveEvaluationSchema(repeated bool) map[string]any {
	properties := map[string]any{"properties": map[string]any{"child": map[string]any{"$ref": "#"}}}
	if !repeated {
		return properties
	}
	return map[string]any{"type": "object", "allOf": []any{properties, properties}}
}
func nestedEvaluationValue(depth int) map[string]any {
	value := map[string]any{}
	for i := 0; i < depth; i++ {
		value = map[string]any{"child": value}
	}
	return value
}

func TestSchemaEvaluationBoundsRecursiveInstanceAmplification(t *testing.T) {
	schema, err := CompileLocalJSONSchema(recursiveEvaluationSchema(true))
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateLocalJSONSchema(schema, nestedEvaluationValue(16)); !errors.Is(err, ErrSchemaEvaluationLimit) {
		t.Fatal("recursive expansion was not bounded", err)
	}
	if err = ValidateLocalJSONSchema(schema, nestedEvaluationValue(4)); err != nil {
		t.Fatal("small recursive data rejected", err)
	}
	linear, err := CompileLocalJSONSchema(recursiveEvaluationSchema(false))
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateLocalJSONSchema(linear, nestedEvaluationValue(40)); err != nil {
		t.Fatal("ordinary recursion rejected", err)
	}
}

func TestNormalizationAndFinalValidationRejectExpensiveCallsAtomically(t *testing.T) {
	input := nestedEvaluationValue(16)
	tools := []any{map[string]any{"name": "walk", "parameters": recursiveEvaluationSchema(true)}}
	calls := []ParsedToolCall{{Name: "walk", Input: input}, {Name: "walk", Input: map[string]any{}}}
	normalized := NormalizeParsedToolCallsForSchemas(calls, tools)
	if !normalized[0].NormalizationLimitExceeded || !normalized[1].NormalizationLimitExceeded || !reflect.DeepEqual(normalized[0].Input, input) {
		t.Fatal("normalization published partial work", normalized)
	}
	if err := ValidateToolArguments(calls, tools); !errors.Is(err, ErrSchemaEvaluationLimit) {
		t.Fatal("final tool validation bypassed evaluation budget", err)
	}
}

func TestFinalBatchSharesSchemaEvaluationBudget(t *testing.T) {
	tools := []any{map[string]any{"name": "walk", "parameters": recursiveEvaluationSchema(true)}}
	call := ParsedToolCall{Name: "walk", Input: nestedEvaluationValue(11)}
	if err := ValidateToolArguments([]ParsedToolCall{call}, tools); err != nil {
		t.Fatal(err)
	}
	calls := []ParsedToolCall{call, call, call, call, call, call, call, call, call, call}
	if err := ValidateToolArguments(calls, tools); !errors.Is(err, ErrSchemaEvaluationLimit) {
		t.Fatal("per-call budget multiplied across a batch", err)
	}
}

func TestEvaluationBudgetUsesAuthoritativeLiteralBeforeParsedShape(t *testing.T) {
	value := nestedEvaluationValue(16)
	encoded, _ := json.Marshal(value)
	properties := map[string]any{"properties": map[string]any{"child": map[string]any{"$ref": "#/$defs/tree"}}}
	schema := map[string]any{"type": "object", "properties": map[string]any{"payload": map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"$ref": "#/$defs/tree"}}}}, "$defs": map[string]any{"tree": map[string]any{"type": "object", "allOf": []any{properties, properties}}}}
	tools := []any{map[string]any{"name": "walk", "parameters": schema}}
	calls := []ParsedToolCall{{Name: "walk", Input: map[string]any{"payload": value}, StringLiterals: map[string]string{"payload": string(encoded)}}}
	normalized := NormalizeParsedToolCallsForSchemas(calls, tools)
	if normalized[0].NormalizationLimitExceeded || normalized[0].Input["payload"] != string(encoded) {
		t.Fatal("literal was budgeted as recursive JSON instead of authoritative text")
	}
	if err := ValidateToolArguments(normalized, tools); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkRecursiveSchemaEvaluationLimit(b *testing.B) {
	schema, err := CompileLocalJSONSchema(recursiveEvaluationSchema(true))
	if err != nil {
		b.Fatal(err)
	}
	value := nestedEvaluationValue(19)
	b.ResetTimer()
	for b.Loop() {
		if err := ValidateLocalJSONSchema(schema, value); !errors.Is(err, ErrSchemaEvaluationLimit) {
			b.Fatal(err)
		}
	}
}
