package toolcall

import (
	"errors"
	"reflect"
	"unsafe"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ErrSchemaEvaluationLimit identifies a workload rejection, not schema validity.
var ErrSchemaEvaluationLimit = errors.New("JSON Schema evaluation exceeds the work or nesting limit")

// ValidateLocalJSONSchema bounds the schema/value product before invoking the
// full validator. Compilation limits alone cannot bound recursive allOf paths
// that revisit the same child instance. No constraints or branches are removed.
func ValidateLocalJSONSchema(schema *jsonschema.Schema, value any) error {
	remaining := maxSchemaEvaluationWork
	return validateSchemaWithBudget(schema, value, &remaining)
}

func validateSchemaWithBudget(schema *jsonschema.Schema, value any, remaining *int) error {
	if schema == nil {
		return nil
	}
	if err := checkSchemaEvaluation(schema, value, remaining); err != nil {
		return err
	}
	return schema.Validate(value)
}

func checkSchemaEvaluation(schema *jsonschema.Schema, value any, remaining *int) error {
	work := schemaEvaluation{remaining: remaining, root: schema}
	return work.visit(schema, value, 0, 0)
}

type schemaEvaluation struct {
	remaining *int
	root      *jsonschema.Schema
	anchors   map[string][]*jsonschema.Schema
	recursive []*jsonschema.Schema
	costs     map[evaluationKey]int
}

// Identity is used only as a local memo key; the pointer is never dereferenced.
// Charge cached expanded cost in full, so memoization cannot bypass the budget.
type evaluationKey struct {
	schema                       *jsonschema.Schema
	value                        unsafe.Pointer
	length, depth, instanceDepth int
}

func (w *schemaEvaluation) charge(n int) error {
	if n > *w.remaining {
		*w.remaining = 0
		return ErrSchemaEvaluationLimit
	}
	*w.remaining -= n
	return nil
}

func (w *schemaEvaluation) visit(s *jsonschema.Schema, value any, depth, instanceDepth int) (err error) {
	if s == nil {
		return nil
	}
	if depth >= 512 || instanceDepth >= 128 {
		return ErrSchemaEvaluationLimit
	}
	if key, ok := evaluationIdentity(s, value, depth, instanceDepth); ok {
		if cost, found := w.costs[key]; found {
			return w.charge(cost)
		}
		before := *w.remaining
		defer func() {
			if err == nil {
				if w.costs == nil {
					w.costs = map[evaluationKey]int{}
				}
				w.costs[key] = before - *w.remaining
			}
		}()
	}
	if err := w.charge(1); err != nil {
		return err
	}
	if s.Bool != nil {
		return nil
	}
	children := sameInstanceSchemas(s)
	// Dynamic scope is owned by the full validator. Count all reachable anchor
	// candidates conservatively instead of assuming the static fallback wins.
	if scopedNormalizationReference(s) {
		if err := w.collectAnchors(); err != nil {
			return err
		}
		targets := w.recursive
		if s.DynamicRef != nil {
			targets = w.anchors[s.DynamicRef.Anchor]
		}
		for _, target := range targets {
			found := false
			for _, child := range children {
				found = found || child == target
			}
			if !found {
				children = append(children, target)
			}
		}
	}
	for _, child := range children {
		if err := w.visit(child, value, depth+1, instanceDepth); err != nil {
			return err
		}
	}
	switch value := value.(type) {
	case map[string]any:
		return w.object(s, value, depth, instanceDepth)
	case []any:
		for i, item := range value {
			if err := w.charge(1); err != nil {
				return err
			}
			for _, child := range []*jsonschema.Schema{compiledItemSchema(s, i), s.Contains, s.UnevaluatedItems} {
				if err := w.visit(child, item, depth+1, instanceDepth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func evaluationIdentity(s *jsonschema.Schema, value any, depth, instanceDepth int) (evaluationKey, bool) {
	length := -1
	switch v := value.(type) {
	case map[string]any:
	case []any:
		length = len(v)
	default:
		return evaluationKey{}, false
	}
	return evaluationKey{schema: s, value: reflect.ValueOf(value).UnsafePointer(), length: length, depth: depth, instanceDepth: instanceDepth}, true
}

func (w *schemaEvaluation) object(s *jsonschema.Schema, object map[string]any, depth, instanceDepth int) error {
	for key, value := range object {
		if err := w.charge(1); err != nil {
			return err
		}
		if err := w.visit(s.PropertyNames, key, depth+1, instanceDepth+1); err != nil {
			return err
		}
		child := s.Properties[key]
		matched := child != nil
		if err := w.visit(child, value, depth+1, instanceDepth+1); err != nil {
			return err
		}
		for pattern, child := range s.PatternProperties {
			if err := w.charge(1); err != nil {
				return err
			}
			if pattern.MatchString(key) {
				matched = true
				if err := w.visit(child, value, depth+1, instanceDepth+1); err != nil {
					return err
				}
			}
		}
		if !matched {
			if child, ok := s.AdditionalProperties.(*jsonschema.Schema); ok {
				if err := w.visit(child, value, depth+1, instanceDepth+1); err != nil {
					return err
				}
			}
		}
		if err := w.visit(s.UnevaluatedProperties, value, depth+1, instanceDepth+1); err != nil {
			return err
		}
	}
	return nil
}

func (w *schemaEvaluation) collectAnchors() error {
	if w.anchors != nil {
		return nil
	}
	w.anchors = map[string][]*jsonschema.Schema{}
	pending := []*jsonschema.Schema{w.root}
	seen := map[*jsonschema.Schema]bool{}
	for len(pending) > 0 {
		s := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if s == nil || seen[s] {
			continue
		}
		if err := w.charge(1); err != nil {
			return err
		}
		seen[s] = true
		if s.DynamicAnchor != "" {
			w.anchors[s.DynamicAnchor] = append(w.anchors[s.DynamicAnchor], s)
		}
		if s.RecursiveAnchor {
			w.recursive = append(w.recursive, s)
		}
		pending = append(pending, sameInstanceSchemas(s)...)
		pending = append(pending, nestedInstanceSchemas(s)...)
	}
	return nil
}

func (r *normalizationRun) validate(s *jsonschema.Schema, value any) error {
	if r.exhausted {
		return ErrSchemaEvaluationLimit
	}
	err := validateSchemaWithBudget(s, value, &r.remaining)
	if errors.Is(err, ErrSchemaEvaluationLimit) {
		r.exhausted = true
	}
	return err
}
