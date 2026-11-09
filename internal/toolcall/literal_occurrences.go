package toolcall

import (
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type parameterLiterals struct {
	single   map[string]string
	repeated map[string]map[int]string
}

func (p *parameterLiterals) record(name string, occurrence int, body string, value any) {
	if occurrence > 0 {
		if raw, ok := p.single[name]; ok {
			p.addRepeated(name, 0, raw)
			delete(p.single, name)
		}
	}
	raw, ok := extractStandaloneCDATA(strings.TrimSpace(body))
	if !ok {
		return
	}
	if text, same := value.(string); same && text == raw {
		return
	}
	if occurrence > 0 {
		p.addRepeated(name, occurrence, raw)
	} else {
		if p.single == nil {
			p.single = map[string]string{}
		}
		p.single[name] = raw
	}
}

func (p *parameterLiterals) addRepeated(name string, index int, raw string) {
	if p.repeated == nil {
		p.repeated = map[string]map[int]string{}
	}
	if p.repeated[name] == nil {
		p.repeated[name] = map[int]string{}
	}
	p.repeated[name][index] = raw
}

func callHasStringLiterals(call ParsedToolCall) bool {
	return len(call.StringLiterals) > 0 || len(call.RepeatedStringLiterals) > 0
}

func literalInputCandidate(call ParsedToolCall) map[string]any {
	input := make(map[string]any, len(call.Input))
	for name, value := range call.Input {
		input[name] = value
	}
	for key, raw := range call.StringLiterals {
		input[key] = raw
	}
	for key, literals := range call.RepeatedStringLiterals {
		values, ok := call.Input[key].([]any)
		if !ok {
			continue
		}
		values = append([]any(nil), values...)
		for i, raw := range literals {
			if i >= 0 && i < len(values) {
				values[i] = raw
			}
		}
		input[key] = values
	}
	return input
}

func restoreRepeatedLiterals(call ParsedToolCall, input map[string]any, schema *jsonschema.Schema, hasSchema, custom bool, run *normalizationRun) map[string]any {
	for key, literals := range call.RepeatedStringLiterals {
		if run.exhausted {
			return call.Input
		}
		values, ok := call.Input[key].([]any)
		if !ok {
			continue
		}
		if input == nil {
			input = make(map[string]any, len(call.Input))
			for name, value := range call.Input {
				input[name] = value
			}
		}
		values = append([]any(nil), values...)
		input[key] = values
		for i, raw := range literals {
			if run.exhausted {
				return call.Input
			}
			if i < 0 || i >= len(values) {
				continue
			}
			previous := values[i]
			values[i] = raw
			if !custom && hasSchema && !propertyItemExpectsString(schema, key, i, input) && (schema == nil || run.validate(schema, input) != nil) {
				values[i] = previous
			}
		}
	}
	return input
}

func propertyItemExpectsString(schema *jsonschema.Schema, key string, index int, values ...any) bool {
	return schemaGraphAny(schema, func(node *jsonschema.Schema) bool {
		accepts := func(child *jsonschema.Schema) bool {
			return schemaGraphAny(child, func(array *jsonschema.Schema) bool {
				return schemaExpectsString(compiledItemSchema(array, index), 0)
			})
		}
		matched := node.Properties[key] != nil
		if accepts(node.Properties[key]) {
			return true
		}
		for pattern, child := range node.PatternProperties {
			if pattern.MatchString(key) {
				matched = true
				if accepts(child) {
					return true
				}
			}
		}
		child, ok := node.AdditionalProperties.(*jsonschema.Schema)
		return !matched && ok && accepts(child)
	}, values...)
}
