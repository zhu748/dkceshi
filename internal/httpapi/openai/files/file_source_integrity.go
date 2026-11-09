package files

import (
	"bytes"
	"fmt"
	"mime"
	"reflect"
	"strings"

	"ds2api/internal/httpapi/openai/shared"
)

var inlineLocatorKeys = []string{"file_data", "base64", "data", "file_url", "url", "file", "source", "inlineData", "inline_data", "image_url"}

// Compare decoded bytes and media types, not transport spelling: padded/raw
// base64 and data URLs may encode the same file. No payload value enters errors.
// The common single-source path keeps its original one-decode behavior.
func validateInlineFileSources(block map[string]any) error {
	kind := strings.ToLower(strings.TrimSpace(shared.AsString(block["type"])))
	var keys []string
	for _, key := range inlineLocatorKeys {
		value, present := block[key]
		if !present {
			continue
		}
		switch key {
		case "source":
			source, ok := value.(map[string]any)
			nativeSource := source["type"] == "base64" || kind == "image" && source["type"] == "url"
			if !ok || (kind != "image" && kind != "document") || !nativeSource {
				continue // Text documents are handled by the text serializer.
			}
		case "file", "inlineData", "inline_data":
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("inline file %s must be an object", key)
			}
		case "image_url":
			switch value.(type) {
			case string, map[string]any:
			default:
				return fmt.Errorf("inline file image_url must be a string or object")
			}
		default:
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return fmt.Errorf("inline file %s must be a non-empty string", key)
			}
		}
		keys = append(keys, key)
	}
	if len(keys) < 2 {
		return nil
	}
	var first inlineDecodedFile
	firstDeclared := false
	for i, key := range keys {
		candidate := make(map[string]any, len(block))
		for name, value := range block {
			candidate[name] = value
		}
		for _, locator := range inlineLocatorKeys {
			delete(candidate, locator)
		}
		candidate[key] = block[key]
		decoded, matched, err := decodeOpenAIInlineFileBlock(candidate)
		if err != nil || !matched {
			return fmt.Errorf("invalid or conflicting inline file source %s", key)
		}
		declared := inlineSourceDeclaresMediaType(candidate)
		if i > 0 && (!bytes.Equal(first.Data, decoded.Data) || firstDeclared && declared && !sameInlineMediaType(first.ContentType, decoded.ContentType)) {
			return fmt.Errorf("conflicting inline file sources %s and %s", keys[0], key)
		}
		if i == 0 || declared && !firstDeclared {
			first, firstDeclared = decoded, declared
		}
	}
	return nil
}

func inlineSourceDeclaresMediaType(block map[string]any) bool {
	if contentTypeFromMap(block) != "" {
		return true
	}
	for _, key := range inlineLocatorKeys {
		switch value := block[key].(type) {
		case map[string]any:
			if inlineSourceDeclaresMediaType(value) {
				return true
			}
		case string:
			value = strings.TrimSpace(value)
			if header, _, ok := strings.Cut(value, ","); ok && isDataURL(value) {
				metadata := header[len("data:"):]
				if media, _, _ := strings.Cut(metadata, ";"); strings.TrimSpace(media) != "" {
					return true
				}
			}
		}
	}
	return false
}

func sameInlineMediaType(first, second string) bool {
	a, aParams, aErr := mime.ParseMediaType(first)
	b, bParams, bErr := mime.ParseMediaType(second)
	if aErr != nil || bErr != nil {
		return strings.EqualFold(strings.TrimSpace(first), strings.TrimSpace(second))
	}
	if charset, ok := aParams["charset"]; ok {
		aParams["charset"] = strings.ToLower(charset)
	}
	if charset, ok := bParams["charset"]; ok {
		bParams["charset"] = strings.ToLower(charset)
	}
	return a == b && reflect.DeepEqual(aParams, bParams)
}
