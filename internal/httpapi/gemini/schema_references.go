package gemini

import (
	"net/url"
	"strconv"
	"strings"
)

type geminiSchemaScope struct{ uri, oldPath, newPath string }
type geminiSchemaReference struct {
	node map[string]any
	uri  string
}
type geminiSchemaReferences struct {
	locations  map[string]map[string]string
	sources    map[string]map[string]any
	references []geminiSchemaReference
}

func newGeminiSchemaReferences() *geminiSchemaReferences {
	return &geminiSchemaReferences{locations: map[string]map[string]string{}, sources: map[string]map[string]any{}}
}

func schemaPointerPart(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func nullableSchemaResourceKey(key string) bool {
	switch key {
	case "$id", "$schema", "$defs", "definitions", "$anchor", "$dynamicAnchor", "$recursiveAnchor":
		return true
	}
	return false
}

// Track each resource as well as pointers from its containing resources. A
// nullable wrapper relocates constraint subtrees, but source $refs still name
// their original JSON Pointers until every relocation is known.
func (r *geminiSchemaReferences) enter(schema map[string]any, oldPath, newPath string, scopes []geminiSchemaScope) []geminiSchemaScope {
	if len(scopes) == 0 {
		scopes = []geminiSchemaScope{{uri: "https://ds2api.invalid/tool-schema.json"}}
		r.sources[scopes[0].uri] = schema
	}
	if id, ok := schema["$id"].(string); ok {
		base, baseErr := url.Parse(scopes[len(scopes)-1].uri)
		ref, refErr := url.Parse(id)
		if baseErr == nil && refErr == nil {
			uri := base.ResolveReference(ref)
			uri.Fragment, uri.RawFragment = "", ""
			r.sources[uri.String()] = schema
			// Allocate a separate slice: siblings must retain their parent's base.
			scopes = append(append([]geminiSchemaScope(nil), scopes...), geminiSchemaScope{uri.String(), oldPath, newPath})
		}
	}
	return scopes
}

func (r *geminiSchemaReferences) location(oldPath, newPath string, scopes []geminiSchemaScope) {
	for _, scope := range scopes {
		locations := r.locations[scope.uri]
		if locations == nil {
			locations = map[string]string{}
			r.locations[scope.uri] = locations
		}
		locations[strings.TrimPrefix(oldPath, scope.oldPath)] = strings.TrimPrefix(newPath, scope.newPath)
	}
}

func (r *geminiSchemaReferences) add(node map[string]any, uri string) {
	for _, key := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
		if _, ok := node[key].(string); ok {
			r.references = append(r.references, geminiSchemaReference{node, uri})
			return
		}
	}
}

func (r *geminiSchemaReferences) rewrite() {
	for _, item := range r.references {
		base, err := url.Parse(item.uri)
		if err != nil {
			continue
		}
		for _, key := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
			text, ok := item.node[key].(string)
			if !ok {
				continue
			}
			ref, err := url.Parse(text)
			if err != nil || !strings.HasPrefix(ref.Fragment, "/") {
				continue
			}
			target := base.ResolveReference(ref)
			target.Fragment, target.RawFragment = "", ""
			locations := r.locations[target.String()]
			if source, known := r.sources[target.String()]; known && !schemaPointerExists(source, ref.Fragment) {
				// A newly inserted anyOf must not make an invalid source pointer
				// resolve to a generated node. Keep it unresolvable at compilation.
				missing := "__ds2api_missing_schema_reference__"
				for {
					if _, exists := source[missing]; !exists {
						break
					}
					missing += "_"
				}
				ref.Fragment, ref.RawFragment = "/"+missing, ""
				item.node[key] = ref.String()
				continue
			}
			// A pointer may target an otherwise unknown schema container. Use
			// its closest tracked parent; annotations themselves are never walked.
			for path := ref.Fragment; ; {
				if moved, found := locations[path]; found {
					next := moved + strings.TrimPrefix(ref.Fragment, path)
					if next != ref.Fragment {
						ref.Fragment, ref.RawFragment = next, ""
						item.node[key] = ref.String()
					}
					break
				}
				end := strings.LastIndexByte(path, '/')
				if end < 0 {
					break
				}
				path = path[:end]
			}
		}
	}
}

func schemaPointerExists(source any, pointer string) bool {
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := source.(type) {
		case map[string]any:
			next, found := current[part]
			if !found {
				return false
			}
			source = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return false
			}
			source = current[index]
		default:
			return false
		}
	}
	return true
}
