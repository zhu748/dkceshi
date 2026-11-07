package prompt

import (
	"sort"
	"strconv"
	"strings"
)

var jsonPointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// ContentOmittedPaths identifies binary/opaque values actually removed by
// content-block serializers. JSON Pointer paths contain no source values.
// Plain JSON object bodies and uploaded references are not reinterpreted.
func ContentOmittedPaths(content any) []string {
	blocks, ok := content.([]any)
	if !ok {
		return nil
	}
	var paths []string
	for i, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok || FileReferenceText(block) != "" {
			continue
		}
		path := "/" + strconv.Itoa(i)
		kind, _ := block["type"].(string)
		var sanitized any
		if preserved, ok := PreserveTextContextBlock(block); ok {
			sanitized = preserved
		} else {
			sanitized = sanitizeContentValue(block)
		}
		if kind == "thinking" || kind == "reasoning" {
			copy := sanitized.(map[string]any)
			for _, key := range []string{"signature", "thoughtSignature", "thought_signature"} {
				if _, present := block[key]; present {
					copy[key] = "[omitted_binary_payload]"
				}
			}
		}
		collectOmittedPaths(block, sanitized, path, &paths)
	}
	sort.Strings(paths)
	return paths
}

func collectOmittedPaths(source, rendered any, path string, paths *[]string) {
	if text, ok := rendered.(string); ok && text == "[omitted_binary_payload]" {
		if original, ok := source.(string); !ok || original != text {
			*paths = append(*paths, path)
		}
		return
	}
	switch source := source.(type) {
	case map[string]any:
		out, _ := rendered.(map[string]any)
		for key, value := range source {
			if next, present := out[key]; present {
				collectOmittedPaths(value, next, path+"/"+jsonPointerEscaper.Replace(key), paths)
			}
		}
	case []any:
		out, _ := rendered.([]any)
		for i, value := range source {
			if i < len(out) {
				collectOmittedPaths(value, out[i], path+"/"+strconv.Itoa(i), paths)
			}
		}
	}
}
