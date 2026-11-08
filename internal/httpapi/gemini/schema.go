package gemini

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Gemini's Schema uses uppercase type enum values and nullable; convert those
// adapter-specific fields before the shared JSON Schema compiler sees them.
func normalizeGeminiSchema(raw any) any {
	refs := newGeminiSchemaReferences()
	out := normalizeGeminiSchemaAt(raw, "", "", nil, refs)
	refs.rewrite()
	return out
}

func normalizeGeminiSchemaAt(raw any, oldPath, newPath string, scopes []geminiSchemaScope, refs *geminiSchemaReferences) any {
	schema, ok := raw.(map[string]any)
	if !ok {
		return raw
	}
	scopes = refs.enter(schema, oldPath, newPath, scopes)
	refs.location(oldPath, newPath, scopes)
	nullable := schema["nullable"] == true
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		childOld, childNew := oldPath+"/"+schemaPointerPart(key), newPath
		if nullable && !nullableSchemaResourceKey(key) {
			childNew += "/anyOf/0"
		}
		childNew += "/" + schemaPointerPart(key)
		refs.location(childOld, childNew, scopes)
		child := func(value any, suffix string) any {
			return normalizeGeminiSchemaAt(value, childOld+suffix, childNew+suffix, scopes, refs)
		}
		switch key {
		case "nullable":
			continue
		case "minItems", "maxItems", "minLength", "maxLength", "minProperties", "maxProperties":
			if text, ok := value.(string); ok {
				if n, err := strconv.ParseInt(text, 10, 64); err == nil && n >= 0 {
					value = json.Number(strconv.FormatInt(n, 10))
				}
			}
		case "enum":
			typ := strings.ToLower(asString(schema["type"]))
			if typ == "integer" || typ == "number" {
				value = normalizeGeminiNumericEnum(value, typ)
			}
		case "type":
			if typ, ok := value.(string); ok {
				value = strings.ToLower(typ)
			}
		case "properties", "$defs", "definitions", "patternProperties", "dependentSchemas":
			if fields, ok := value.(map[string]any); ok {
				next := make(map[string]any, len(fields))
				for name, child := range fields {
					next[name] = normalizeGeminiSchemaAt(child, childOld+"/"+schemaPointerPart(name), childNew+"/"+schemaPointerPart(name), scopes, refs)
				}
				value = next
			}
		case "items", "additionalItems", "additionalProperties", "not", "contains", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema":
			value = child(value, "")
		case "anyOf", "oneOf", "allOf", "prefixItems":
			if choices, ok := value.([]any); ok {
				next := make([]any, len(choices))
				for i, choice := range choices {
					next[i] = child(choice, "/"+strconv.Itoa(i))
				}
				value = next
			}
		}
		out[key] = value
	}
	refs.add(out, scopes[len(scopes)-1].uri)
	if nullable {
		nullable := map[string]any{"anyOf": []any{out, map[string]any{"type": "null"}}}
		// Resource identities and reusable definitions stay on the outer node.
		for key, value := range out {
			if nullableSchemaResourceKey(key) {
				nullable[key] = value
				delete(out, key)
			}
		}
		return nullable
	}
	return out
}

func normalizeGeminiNumericEnum(raw any, typ string) any {
	values, ok := raw.([]any)
	if !ok {
		return raw
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" && (text[0] == '-' || text[0] >= '0' && text[0] <= '9') && json.Valid([]byte(text)) {
				if typ == "integer" {
					if _, err := strconv.ParseInt(text, 10, 64); err != nil {
						continue
					}
				} else {
					if _, err := strconv.ParseFloat(text, 64); err != nil {
						continue
					}
					if e := strings.IndexAny(text, "eE"); e >= 0 {
						exponent, err := strconv.ParseInt(text[e+1:], 10, 16)
						if err != nil || exponent < -308 || exponent > 308 {
							continue
						}
					}
				}
				out[i] = json.Number(text)
			}
		}
	}
	return out
}
