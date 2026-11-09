package toolcall

import (
	"reflect"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func (r *normalizationRun) hasConditions(schema *jsonschema.Schema) bool {
	if result, ok := r.conditions[schema]; ok {
		return result
	}
	seen := map[*jsonschema.Schema]bool{}
	pending := []*jsonschema.Schema{schema}
	found := false
	for len(pending) > 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if node == nil || seen[node] {
			continue
		}
		if r.remaining <= 0 {
			r.exhausted = true
			return true
		}
		r.remaining--
		seen[node] = true
		if node.If != nil || scopedNormalizationReference(node) {
			found = true
			break
		}
		pending = append(pending, sameInstanceSchemas(node)...)
		pending = append(pending, nestedInstanceSchemas(node)...)
	}
	if r.conditions == nil {
		r.conditions = map[*jsonschema.Schema]bool{}
	}
	r.conditions[schema] = found
	return found
}

// Check the original/candidate pair once at the whole-value boundary. Checking
// only inside conditional() misses conversions made by properties, references
// or earlier allOf siblings, including constraints on nested instances.
func (r *normalizationRun) preserveConditions(before, after any, schema *jsonschema.Schema, depth int, seen map[*jsonschema.Schema]bool) bool {
	if schema == nil || seen[schema] {
		return true
	}
	if !r.hasConditions(schema) {
		return !r.exhausted
	}
	if r.remaining <= 0 || depth >= 128 {
		r.exhausted = true
		return false
	}
	r.remaining--
	if seen == nil {
		seen = map[*jsonschema.Schema]bool{}
	}
	seen[schema] = true
	if scopedNormalizationReference(schema) {
		// Dynamic anchor selection is owned by the validator. Without its
		// private resource scope, conditional coercion cannot prove stability.
		return reflect.DeepEqual(before, after)
	}
	if schema.If != nil && (r.validate(schema.If, before) == nil) != (r.validate(schema.If, after) == nil) {
		return false
	}
	children := []*jsonschema.Schema{schema.Ref, staticNormalizationReference(schema), schema.Not, schema.If}
	for _, group := range [][]*jsonschema.Schema{schema.AllOf, selectConditionalSchemas(schema, before, r.validate)} {
		children = append(children, group...)
	}
	for _, group := range [][]*jsonschema.Schema{schema.AnyOf, schema.OneOf} {
		for _, child := range group {
			// An alternative invalid both before and after is not the branch
			// being recovered. It cannot justify accepting the candidate.
			if r.validate(child, before) == nil || r.validate(child, after) == nil {
				children = append(children, child)
			}
		}
	}
	for _, child := range children {
		if !r.preserveConditions(before, after, child, depth+1, seen) {
			return false
		}
	}
	switch previous := before.(type) {
	case map[string]any:
		next, ok := after.(map[string]any)
		if !ok {
			return false // Do not erase nested conditional constraints by coercing the container.
		}
		for key, value := range previous {
			if !r.preservePropertyConditions(value, next[key], key, schema, depth+1) {
				return false
			}
		}
	case []any:
		next, ok := after.([]any)
		if !ok || len(next) != len(previous) {
			return false
		}
		for i, value := range previous {
			for _, child := range []*jsonschema.Schema{compiledItemSchema(schema, i), schema.Contains, schema.UnevaluatedItems} {
				if !r.preserveConditions(value, next[i], child, depth+1, nil) {
					return false
				}
			}
		}
	}
	return true
}

func (r *normalizationRun) preservePropertyConditions(before, after any, key string, schema *jsonschema.Schema, depth int) bool {
	matched := schema.Properties[key] != nil
	if !r.preserveConditions(before, after, schema.Properties[key], depth, nil) {
		return false
	}
	for pattern, child := range schema.PatternProperties {
		if pattern.MatchString(key) {
			matched = true
			if !r.preserveConditions(before, after, child, depth, nil) {
				return false
			}
		}
	}
	if !matched {
		if child, ok := schema.AdditionalProperties.(*jsonschema.Schema); ok && !r.preserveConditions(before, after, child, depth, nil) {
			return false
		}
	}
	return r.preserveConditions(before, after, schema.UnevaluatedProperties, depth, nil)
}
