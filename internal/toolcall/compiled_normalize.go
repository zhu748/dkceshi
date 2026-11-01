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
	if value == nil || schema == nil || depth >= 128 {
		return value, false
	}
	changed := false
	apply := func(child *jsonschema.Schema) {
		next, didChange := normalizeCompiledToolValue(value, child, depth+1)
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
	}
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, item := range current {
			next, didChange := normalizeCompiledProperty(item, key, schema, depth+1)
			out[key], changed = next, changed || didChange
		}
		if changed {
			value = out
		}
	case []any:
		out := make([]any, len(current))
		for i, item := range current {
			child := compiledItemSchema(schema, i)
			next, didChange := normalizeCompiledToolValue(item, child, depth+1)
			out[i], changed = next, changed || didChange
		}
		if changed {
			value = out
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
			next, didChange := normalizeCompiledToolValue(value, child, depth+1)
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
func normalizeCompiledProperty(value any, key string, schema *jsonschema.Schema, depth int) (any, bool) {
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
	for _, child := range children {
		next, didChange := normalizeCompiledToolValue(value, child, depth)
		value, changed = next, changed || didChange
	}
	return value, changed
}
