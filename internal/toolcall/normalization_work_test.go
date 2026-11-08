package toolcall

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func sharedReferenceSchema(depth int) map[string]any {
	defs := map[string]any{"n0": map[string]any{"type": "number"}}
	for i := 1; i <= depth; i++ {
		ref := map[string]any{"$ref": fmt.Sprintf("#/$defs/n%d", i-1)}
		defs[fmt.Sprintf("n%d", i)] = map[string]any{"allOf": []any{ref, ref}}
	}
	return map[string]any{"type": "object", "$defs": defs, "properties": map[string]any{
		"value": map[string]any{"$ref": fmt.Sprintf("#/$defs/n%d", depth)},
	}}
}

func TestSharedReferencesNormalizeWithoutRepeatedExpansion(t *testing.T) {
	schema, err := compileToolSchema(sharedReferenceSchema(12))
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"value": float64(1)}
	run := normalizationRun{remaining: maxNormalizationSteps}
	got, changed := run.root(input, schema, 0)
	if changed || !reflect.DeepEqual(got, input) || run.exhausted {
		t.Fatal("unchanged shared references changed input")
	}
	if work := maxNormalizationSteps - run.remaining; work > 100 {
		t.Fatalf("shared references were expanded repeatedly: steps=%d", work)
	}
	if err := schema.Validate(got); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizationBudgetDoesNotReturnPartialCoercions(t *testing.T) {
	schema, err := compileToolSchema(map[string]any{"properties": map[string]any{
		"first": map[string]any{"type": "string"}, "second": map[string]any{"type": "string"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"first": 1, "second": 2}
	run := normalizationRun{remaining: 2}
	got, changed := run.root(input, schema, 0)
	if changed || !run.exhausted || !reflect.DeepEqual(got, map[string]any{"first": 1, "second": 2}) || !reflect.DeepEqual(input, got) {
		t.Fatalf("budget exhaustion exposed partial input: %#v", got)
	}
}

func TestExcessiveNormalizationCannotBecomeAnExecutableCall(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"child": map[string]any{"$ref": "#"}, "value": map[string]any{"type": "string"},
	}}
	tools := []any{map[string]any{"name": "work", "parameters": schema}}
	input := map[string]any{"value": 1}
	for range 100 {
		input = map[string]any{"value": 1, "child": input}
	}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "work", Input: input}}, tools)
	if !calls[0].NormalizationLimitExceeded || !reflect.DeepEqual(calls[0].Input, input) || input["value"] != 1 {
		t.Fatal("excessive normalization lost its failure or partially changed arguments")
	}
	if err := ValidateToolArguments(calls, tools); err == nil || !strings.Contains(err.Error(), "work or nesting limit") {
		t.Fatalf("excessive normalization was not rejected explicitly: %v", err)
	}
}

func TestNormalizationCacheSeparatesValuesAndInvalidatesAfterCoercion(t *testing.T) {
	source := map[string]any{"$defs": map[string]any{"number": map[string]any{"type": "number"}},
		"$ref": "#/$defs/number", "type": "string", "allOf": []any{map[string]any{"$ref": "#/$defs/number"}}}
	schema, err := compileToolSchema(source)
	if err != nil {
		t.Fatal(err)
	}
	got, changed := normalizeCompiledToolValue(float64(7), schema, 0)
	if !changed || got != float64(7) {
		t.Fatalf("cached number result survived intervening string coercion: %#v", got)
	}
	if schema.Validate(got) == nil {
		t.Fatal("contradictory schema became valid")
	}
	tools := []any{map[string]any{"name": "write", "parameters": map[string]any{"$defs": map[string]any{"text": map[string]any{"type": "string"}},
		"properties": map[string]any{"a": map[string]any{"$ref": "#/$defs/text"}, "b": map[string]any{"$ref": "#/$defs/text"}}}}}
	calls := NormalizeParsedToolCallsForSchemas([]ParsedToolCall{{Name: "write", Input: map[string]any{"a": 1, "b": 2}}}, tools)
	if calls[0].Input["a"] != "1" || calls[0].Input["b"] != "2" {
		t.Fatalf("shared schema mixed distinct values: %#v", calls)
	}
}

func TestSchemaWorkRejectsExpansionWithoutChangingSchemaSemantics(t *testing.T) {
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		source := sharedReferenceSchema(22)
		defs := source["$defs"].(map[string]any)
		if keyword != "allOf" {
			for i := 1; i <= 22; i++ {
				node := defs[fmt.Sprintf("n%d", i)].(map[string]any)
				node[keyword] = node["allOf"]
				delete(node, "allOf")
			}
		}
		before, _ := json.Marshal(source)
		tools := []any{map[string]any{"name": "work", "parameters": source}}
		if err := ValidateToolSchemas(tools); err == nil || !strings.Contains(err.Error(), "complexity limit") {
			t.Fatalf("expanded %s schema was not rejected explicitly: %v", keyword, err)
		}
		after, _ := json.Marshal(source)
		if string(before) != string(after) {
			t.Fatal("schema work check rewrote source context")
		}
	}
	duplicate := map[string]any{"$defs": map[string]any{"text": map[string]any{"type": "string"}}, "oneOf": []any{
		map[string]any{"$ref": "#/$defs/text"}, map[string]any{"$ref": "#/$defs/text"},
	}}
	schema, err := compileToolSchema(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Validate("two matches") == nil {
		t.Fatal("oneOf multiplicity was weakened by reference optimization")
	}
}

func BenchmarkSharedReferenceNormalization(b *testing.B) {
	// Bypass the admission check to measure the original audit's 22-level DAG.
	c := jsonschema.NewCompiler()
	const resource = "https://ds2api.invalid/benchmark.json"
	if err := c.AddResource(resource, sharedReferenceSchema(22)); err != nil {
		b.Fatal(err)
	}
	schema, err := c.Compile(resource)
	if err != nil {
		b.Fatal(err)
	}
	input := map[string]any{"value": float64(1)}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, changed := normalizeCompiledToolValue(input, schema, 0); changed {
			b.Fatal("unchanged input changed")
		}
	}
}
