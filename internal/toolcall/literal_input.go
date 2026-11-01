package toolcall

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
	"strings"
)

// Keep a CDATA string's bytes even when the compatibility parser interpreted
// them as JSON first. Schema constraints choose types, never rewrite strings.
func restoreLiteralInput(call ParsedToolCall, schema any, custom bool) map[string]any {
	if len(call.StringLiterals) == 0 {
		return call.Input
	}
	compiled, _ := compileToolSchema(schema)
	var input map[string]any
	for key, raw := range call.StringLiterals {
		if !custom && !propertyExpectsString(compiled, key, 0) {
			continue
		}
		if input == nil {
			input = make(map[string]any, len(call.Input))
			for name, value := range call.Input {
				input[name] = value
			}
		}
		input[key] = raw
	}
	if input != nil {
		return input
	}
	return call.Input
}

// References can form cycles or shared DAGs; visit each compiled node once.
func propertyExpectsString(schema *jsonschema.Schema, key string, _ int) bool {
	return schemaGraphAny(schema, func(node *jsonschema.Schema) bool {
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
	})
}

func schemaExpectsString(schema *jsonschema.Schema, _ int) bool {
	return schemaGraphAny(schema, func(node *jsonschema.Schema) bool {
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

func schemaGraphAny(schema *jsonschema.Schema, match func(*jsonschema.Schema) bool) bool {
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
		pending = append(pending, node.AllOf...)
		pending = append(pending, node.AnyOf...)
		pending = append(pending, node.OneOf...)
	}
	return false
}

func recordStringLiteral(literals map[string]string, input map[string]any, name, body string, value any) {
	if _, duplicate := input[name]; duplicate {
		delete(literals, name)
		return
	}
	if raw, ok := extractStandaloneCDATA(strings.TrimSpace(body)); ok {
		if text, sameType := value.(string); !sameType || text != raw {
			literals[name] = raw
		}
	}
}
