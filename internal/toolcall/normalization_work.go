package toolcall

import "github.com/santhosh-tekuri/jsonschema/v6"

const maxNormalizationSteps = 1 << 18

// One budget covers a whole call batch. A failed traversal returns its original
// input atomically; a partly coerced object must never escape this boundary.
type normalizationRun struct {
	remaining  int
	exhausted  bool
	conditions map[*jsonschema.Schema]bool
}

func (r *normalizationRun) root(value any, schema *jsonschema.Schema, depth int) (any, bool) {
	var scope normalizationScope
	next, changed := r.walk(value, schema, depth, &scope)
	if changed && r.hasConditions(schema) && !r.preserveConditions(value, next, schema, depth, nil) {
		return value, false
	}
	if r.exhausted {
		return value, false
	}
	return next, changed
}

// References and compositions share a scope only while evaluating the same
// value. Coercions advance its version; nested values and trial alternatives
// get separate scopes. Cache only unchanged results, never a guessed fixed point.
type normalizationScope struct {
	version uint64
	seen    map[*jsonschema.Schema]uint64
}

func (s *normalizationScope) cached(schema *jsonschema.Schema) bool {
	version, ok := s.seen[schema]
	return ok && version == s.version
}

func (s *normalizationScope) begin(schema *jsonschema.Schema) {
	if s.seen == nil && (schema.Ref != nil || len(schema.AllOf)+len(schema.AnyOf)+len(schema.OneOf) > 0) {
		s.seen = make(map[*jsonschema.Schema]uint64)
	}
}

func (s *normalizationScope) finish(schema *jsonschema.Schema, version uint64, changed bool) {
	if changed {
		s.version++
	} else if s.seen != nil && version == s.version {
		s.seen[schema] = version
	}
}
