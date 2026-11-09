package toolcall

import (
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Select branches from the existing value, never by trying to turn a failed
// discriminator into a different branch. Dependent schemas run in sorted order.
func activeConditionalSchemas(schema *jsonschema.Schema, value any) []*jsonschema.Schema {
	return selectConditionalSchemas(schema, value, ValidateLocalJSONSchema)
}

func selectConditionalSchemas(schema *jsonschema.Schema, value any, validate func(*jsonschema.Schema, any) error) []*jsonschema.Schema {
	var children []*jsonschema.Schema
	if schema.If != nil {
		if validate(schema.If, value) == nil {
			children = append(children, schema.Then)
		} else {
			children = append(children, schema.Else)
		}
	}
	object, ok := value.(map[string]any)
	if !ok {
		return children
	}
	keys := make([]string, 0, len(schema.DependentSchemas)+len(schema.Dependencies))
	for key := range schema.DependentSchemas {
		keys = append(keys, key)
	}
	for key := range schema.Dependencies {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, present := object[key]; !present {
			continue
		}
		if child := schema.DependentSchemas[key]; child != nil {
			children = append(children, child)
		}
		if child, ok := schema.Dependencies[key].(*jsonschema.Schema); ok {
			children = append(children, child)
		}
	}
	return children
}

func (r *normalizationRun) conditional(value any, schema *jsonschema.Schema, depth int) (any, bool) {
	if schema.If == nil && len(schema.DependentSchemas)+len(schema.Dependencies) == 0 {
		return value, false
	}
	children := selectConditionalSchemas(schema, value, r.validate)
	if len(children) == 0 || r.validate(schema, value) == nil {
		return value, false
	}
	next, changed := value, false
	var scope normalizationScope
	for _, child := range children {
		candidate, didChange := r.walk(next, child, depth+1, &scope)
		next, changed = candidate, changed || didChange
		if r.exhausted {
			return value, false
		}
	}
	if !changed {
		return value, false
	}
	// Coercion must preserve the selected condition and satisfy the whole node.
	if schema.If != nil && (r.validate(schema.If, value) == nil) != (r.validate(schema.If, next) == nil) {
		return value, false
	}
	if r.validate(schema, next) != nil {
		return value, false
	}
	return next, true
}
