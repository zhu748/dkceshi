package toolcall

import (
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const maxSchemaNodes = 32768
const maxSchemaEvaluationWork = 1 << 16

// The validator evaluates repeated references separately. Bound their expanded
// work before publishing a compiled schema, even when its source is tiny. Value
// descent (properties/items) starts a new instance, so ordinary recursive data
// schemas remain supported. Never rewrite oneOf or change reference locations.
func checkSchemaWork(root *jsonschema.Schema) error {
	type schemaCost struct{ work, height int }
	costs := map[*jsonschema.Schema]schemaCost{}
	active := map[*jsonschema.Schema]bool{}
	var cost func(*jsonschema.Schema, int) (int, error)
	cost = func(schema *jsonschema.Schema, depth int) (int, error) {
		if schema == nil {
			return 0, nil
		}
		if n, ok := costs[schema]; ok {
			if depth+n.height > 128 {
				return 0, fmt.Errorf("JSON Schema exceeds the same-instance reference depth limit")
			}
			return n.work, nil
		}
		if active[schema] || depth >= 128 {
			return 0, fmt.Errorf("JSON Schema exceeds the same-instance reference depth limit")
		}
		active[schema] = true
		n := 1
		height := 1
		for _, child := range sameInstanceSchemas(schema) {
			childCost, err := cost(child, depth+1)
			if err != nil {
				return 0, err
			}
			n += childCost
			height = max(height, 1+costs[child].height)
			if n > maxSchemaEvaluationWork {
				return 0, fmt.Errorf("JSON Schema exceeds the evaluation complexity limit")
			}
		}
		delete(active, schema)
		costs[schema] = schemaCost{work: n, height: height}
		return n, nil
	}
	seen := map[*jsonschema.Schema]bool{}
	pending := []*jsonschema.Schema{root}
	for len(pending) > 0 {
		schema := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if schema == nil || seen[schema] {
			continue
		}
		seen[schema] = true
		if len(seen) > maxSchemaNodes {
			return fmt.Errorf("JSON Schema exceeds the compiled node limit")
		}
		if _, err := cost(schema, 0); err != nil {
			return err
		}
		pending = append(pending, sameInstanceSchemas(schema)...)
		pending = append(pending, nestedInstanceSchemas(schema)...)
	}
	return nil
}

func sameInstanceSchemas(s *jsonschema.Schema) []*jsonschema.Schema {
	out := []*jsonschema.Schema{s.Ref, s.RecursiveRef, s.Not, s.If, s.Then, s.Else}
	if s.DynamicRef != nil {
		out = append(out, s.DynamicRef.Ref)
	}
	for _, children := range [][]*jsonschema.Schema{s.AllOf, s.AnyOf, s.OneOf} {
		out = append(out, children...)
	}
	for _, child := range s.DependentSchemas {
		out = append(out, child)
	}
	for _, raw := range s.Dependencies {
		if child, ok := raw.(*jsonschema.Schema); ok {
			out = append(out, child)
		}
	}
	return out
}

func nestedInstanceSchemas(s *jsonschema.Schema) []*jsonschema.Schema {
	out := []*jsonschema.Schema{s.PropertyNames, s.UnevaluatedProperties, s.Contains, s.Items2020, s.UnevaluatedItems, s.ContentSchema}
	for _, child := range s.Properties {
		out = append(out, child)
	}
	for _, child := range s.PatternProperties {
		out = append(out, child)
	}
	for _, raw := range []any{s.AdditionalProperties, s.Items, s.AdditionalItems} {
		switch children := raw.(type) {
		case *jsonschema.Schema:
			out = append(out, children)
		case []*jsonschema.Schema:
			out = append(out, children...)
		}
	}
	return append(out, s.PrefixItems...)
}
