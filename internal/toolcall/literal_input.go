package toolcall

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Keep a CDATA string's bytes even when the compatibility parser interpreted
// them as JSON first. Schema constraints choose types, never rewrite strings.
func restoreLiteralInput(call ParsedToolCall, compiled *jsonschema.Schema, hasSchema, custom bool, run *normalizationRun) map[string]any {
	if !callHasStringLiterals(call) {
		return call.Input
	}
	// Restore a valid literal batch atomically. Conditional schemas can require
	// multiple fields/items to change together; map iteration cannot choose types.
	preferred := literalInputCandidate(call)
	if custom || !hasSchema || compiled != nil && run.validate(compiled, preferred) == nil {
		return preferred
	}
	if run.exhausted {
		return call.Input
	}
	var input map[string]any
	for key, raw := range call.StringLiterals {
		if run.exhausted {
			return call.Input
		}
		if input == nil {
			input = make(map[string]any, len(call.Input))
			for name, value := range call.Input {
				input[name] = value
			}
		}
		previous := input[key]
		input[key] = raw
		if !custom && hasSchema && !propertyExpectsString(compiled, key, 0, input) && (compiled == nil || run.validate(compiled, input) != nil) {
			input[key] = previous
		}
	}
	input = restoreRepeatedLiterals(call, input, compiled, hasSchema, custom, run)
	if input != nil {
		return input
	}
	return call.Input
}

// References can form cycles or shared DAGs; visit each compiled node once.
func propertyExpectsString(schema *jsonschema.Schema, key string, _ int, values ...any) bool {
	allows := schemaGraphAny(schema, func(node *jsonschema.Schema) bool {
		if child := node.Properties[key]; child != nil && schemaExpectsString(child, 0) {
			return true
		}
		matched := node.Properties[key] != nil
		for pattern, child := range node.PatternProperties {
			if pattern.MatchString(key) {
				matched = true
				if schemaExpectsString(child, 0) {
					return true
				}
			}
		}
		child, ok := node.AdditionalProperties.(*jsonschema.Schema)
		return !matched && ok && schemaExpectsString(child, 0)
	}, values...)
	return allows
}

func schemaExpectsString(schema *jsonschema.Schema, _ int) bool {
	return schemaGraphAny(schema, func(node *jsonschema.Schema) bool {
		if node.Bool != nil {
			return *node.Bool
		}
		if node.Types == nil && node.Const == nil && node.Enum == nil && node.Ref == nil && staticNormalizationReference(node) == nil && node.Not == nil && len(node.AllOf) == 0 && len(node.AnyOf) == 0 && len(node.OneOf) == 0 {
			return true
		}
		if node.Types != nil {
			for _, typ := range node.Types.ToStrings() {
				if typ == "string" {
					return true
				}
			}
		}
		if node.Const != nil {
			if _, ok := (*node.Const).(string); ok {
				return true
			}
		}
		if node.Enum != nil {
			for _, value := range node.Enum.Values {
				if _, ok := value.(string); ok {
					return true
				}
			}
		}
		return false
	})
}

func schemaGraphAny(schema *jsonschema.Schema, match func(*jsonschema.Schema) bool, values ...any) bool {
	pending := []*jsonschema.Schema{schema}
	seen := map[*jsonschema.Schema]bool{}
	for len(pending) > 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if node == nil || seen[node] {
			continue
		}
		seen[node] = true
		if match(node) {
			return true
		}
		pending = append(pending, node.Ref)
		pending = append(pending, staticNormalizationReference(node))
		pending = append(pending, node.AllOf...)
		pending = append(pending, node.AnyOf...)
		pending = append(pending, node.OneOf...)
		if len(values) > 0 {
			pending = append(pending, activeConditionalSchemas(node, values[0])...)
		}
	}
	return false
}
