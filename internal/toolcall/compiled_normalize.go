package toolcall

import (
	"reflect"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Walk the compiler's local reference graph, so coercion and validation agree
// on $ref/$id/anchors and recursive schemas. Bound schema-only cycles as well
// as deeply nested values; final validation still decides whether to deliver.
func normalizeCompiledToolValue(value any, schema *jsonschema.Schema, depth int) (any, bool) {
	run := normalizationRun{remaining: maxNormalizationSteps}
	return run.root(value, schema, depth)
}

func (r *normalizationRun) walk(value any, schema *jsonschema.Schema, depth int, scope *normalizationScope) (next any, changed bool) {
	if value == nil || schema == nil || scope.cached(schema) {
		return value, false
	}
	if r.exhausted || r.remaining <= 0 || depth >= 128 {
		r.exhausted = true
		return value, false
	}
	r.remaining--
	scope.begin(schema)
	version := scope.version
	defer func() { scope.finish(schema, version, changed) }()
	if text, ok := value.(string); ok && !schemaExpectsString(schema, 0) && schema.Validate(text) != nil {
		if parsed, ok := parseJSONLiteralValue(text); ok && schema.Validate(parsed) == nil {
			return parsed, true
		}
		if parsed, ok := parseArrayMissingClosures(text); ok && schema.Validate(parsed) == nil {
			return parsed, true
		}
	}
	apply := func(child *jsonschema.Schema) {
		next, didChange := r.walk(value, child, depth+1, scope)
		value, changed = next, changed || didChange
	}
	if schema.Ref != nil {
		apply(schema.Ref)
	}
	stringSchema := map[string]any{}
	if schema.Types != nil {
		stringSchema["type"] = schema.Types.ToStrings()
	}
	if schema.Const != nil {
		stringSchema["const"] = *schema.Const
	}
	if schema.Enum != nil {
		stringSchema["enum"] = schema.Enum.Values
	}
	if shouldCoerceSchemaToString(stringSchema) {
		next, didChange := stringifySchemaValue(value)
		value, changed = next, changed || didChange
		if didChange {
			scope.version++
		}
	}
	switch current := value.(type) {
	case map[string]any:
		var out map[string]any
		for key, item := range current {
			next, didChange := r.property(item, key, schema, depth+1)
			if didChange {
				if out == nil {
					out = make(map[string]any, len(current))
					for name, original := range current {
						out[name] = original
					}
				}
				out[key] = next
			}
		}
		if out != nil {
			value, changed = out, true
			scope.version++
		}
	case []any:
		var out []any
		for i, item := range current {
			child := compiledItemSchema(schema, i)
			var childScope normalizationScope
			next, didChange := r.walk(item, child, depth+1, &childScope)
			if didChange {
				if out == nil {
					out = append([]any(nil), current...)
				}
				out[i] = next
			}
		}
		if out != nil {
			value, changed = out, true
			scope.version++
		}
	}
	for _, child := range schema.AllOf {
		apply(child)
	}
	for _, alternatives := range [][]*jsonschema.Schema{schema.AnyOf, schema.OneOf} {
		if len(alternatives) == 0 || schema.Validate(value) == nil {
			continue
		}
		var candidate any
		found, ambiguous := false, false
		for _, child := range alternatives {
			var trial normalizationScope
			next, didChange := r.walk(value, child, depth+1, &trial)
			if !didChange || schema.Validate(next) != nil {
				continue
			}
			if found && !reflect.DeepEqual(candidate, next) {
				ambiguous = true
				break
			}
			candidate, found = next, true
		}
		if found && !ambiguous {
			value, changed = candidate, true
			scope.version++
		}
	}
	return value, changed
}

func compiledItemSchema(schema *jsonschema.Schema, index int) *jsonschema.Schema {
	if index < len(schema.PrefixItems) {
		return schema.PrefixItems[index]
	}
	if schema.Items2020 != nil {
		return schema.Items2020
	}
	switch items := schema.Items.(type) {
	case *jsonschema.Schema:
		return items
	case []*jsonschema.Schema:
		if index < len(items) {
			return items[index]
		}
		additional, _ := schema.AdditionalItems.(*jsonschema.Schema)
		return additional
	}
	return nil
}

// properties and every matching pattern apply together. Additional properties
// only applies when neither keyword evaluated the property.
func (r *normalizationRun) property(value any, key string, schema *jsonschema.Schema, depth int) (any, bool) {
	children := []*jsonschema.Schema{}
	if child := schema.Properties[key]; child != nil {
		children = append(children, child)
	}
	patterns := make([]jsonschema.Regexp, 0, len(schema.PatternProperties))
	for pattern := range schema.PatternProperties {
		if pattern.MatchString(key) {
			patterns = append(patterns, pattern)
		}
	}
	sort.Slice(patterns, func(i, j int) bool { return patterns[i].String() < patterns[j].String() })
	for _, pattern := range patterns {
		children = append(children, schema.PatternProperties[pattern])
	}
	if len(children) == 0 {
		if child, ok := schema.AdditionalProperties.(*jsonschema.Schema); ok {
			children = append(children, child)
		}
	}
	changed := false
	var scope normalizationScope
	for _, child := range children {
		next, didChange := r.walk(value, child, depth, &scope)
		value, changed = next, changed || didChange
	}
	return value, changed
}
