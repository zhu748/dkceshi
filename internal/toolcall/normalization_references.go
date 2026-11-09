package toolcall

import "github.com/santhosh-tekuri/jsonschema/v6"

func scopedNormalizationReference(schema *jsonschema.Schema) bool {
	return schema.DynamicRef != nil && schema.DynamicRef.Anchor != "" && schema.DynamicRef.Ref.DynamicAnchor == schema.DynamicRef.Anchor ||
		schema.RecursiveRef != nil && schema.RecursiveRef.RecursiveAnchor
}

// Pointer references and references to ordinary anchors have fixed targets.
// Named dynamic/recursive anchors require the validator's resource scope,
// which is private to the compiler: do not guess a target for type coercion.
// Final validation always uses the complete compiled schema and dynamic scope.
func staticNormalizationReference(schema *jsonschema.Schema) *jsonschema.Schema {
	if ref := schema.DynamicRef; ref != nil && (ref.Anchor == "" || ref.Ref.DynamicAnchor != ref.Anchor) {
		return ref.Ref
	}
	if ref := schema.RecursiveRef; ref != nil && !ref.RecursiveAnchor {
		return ref
	}
	return nil
}
