package sse

import (
	"strings"
)

func collectDirectFragments(path string, chunk map[string]any, v any, thinkingEnabled bool, newType *string, parts *[]ContentPart) {
	if path != "response/fragments" {
		return
	}
	op, _ := chunk["o"].(string)
	if !strings.EqualFold(op, "APPEND") {
		return
	}
	frags, ok := v.([]any)
	if !ok {
		return
	}
	for _, frag := range frags {
		m, ok := frag.(map[string]any)
		if !ok {
			continue
		}
		typeName, content, fragType := parseFragmentTypeContent(m)
		if typeName == "" {
			typeName = fragType
		}
		switch typeName {
		case "THINK", "THINKING":
			if !bodyStarted(thinkingEnabled, *newType) {
				// THINK fragment moves the cursor to thinking (see
				// appendWrappedFragments); RESPONSE is the only way back to
				// text. Once the body has begun (one-size-fits-all rule) the
				// cursor never rewinds; the part is still appended so the
				// caller's detection layer sees the upstream activity, then
				// ParseSSEChunkForContentDetailed drops it from visible parts.
				*newType = "thinking"
			}
			appendContentPart(parts, content, "thinking")
		case "RESPONSE":
			*newType = "text"
			appendContentPart(parts, content, "text")
		default:
			if isToolFragmentType(typeName) {
				appendContentPart(parts, content, "thinking")
			} else {
				appendContentPart(parts, content, "text")
			}
		}
	}
}

func updateTypeFromNestedResponse(path string, v any, thinkingEnabled bool, newType *string) {
	if path != "response" {
		return
	}
	arr, ok := v.([]any)
	if !ok {
		return
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok || m["p"] != "fragments" || m["o"] != "APPEND" {
			continue
		}
		frags, ok := m["v"].([]any)
		if !ok {
			continue
		}
		for _, frag := range frags {
			fm, ok := frag.(map[string]any)
			if !ok {
				continue
			}
			typeName, _, _ := parseFragmentTypeContent(fm)
			switch typeName {
			case "THINK", "THINKING":
				if !bodyStarted(thinkingEnabled, *newType) {
					// THINK fragment moves the cursor to thinking; RESPONSE is
					// the only way back to text. Once the body has begun
					// (one-size-fits-all rule) the cursor never rewinds.
					*newType = "thinking"
				}
			case "RESPONSE":
				*newType = "text"
			}
		}
	}
}

func resolvePartType(path string, thinkingEnabled bool, newType string) string {
	switch {
	case path == "response/thinking_content":
		// Reasoning content is always typed "thinking". When the body has
		// already begun, ParseSSEChunkForContentDetailed drops it wholesale
		// (the one-size-fits-all rule) instead of the old behavior that
		// recovered it as body text and leaked reasoning into the answer.
		return "thinking"
	case path == "response/content":
		return "text"
	case (strings.HasPrefix(path, "fragments/") || strings.HasPrefix(path, "response/fragments/")) && strings.Contains(path, "/content"):
		// fragment content paths follow the sticky cursor. DeepSeek emits the
		// relative form "fragments/-1/content" (no "response/" prefix) inside a
		// response BATCH; it carries the current stream kind, not a body marker.
		return newType
	case path == "":
		if newType != "" {
			return newType
		}
		return "text"
	default:
		return "text"
	}
}

func appendChunkValueContent(v any, partType string, thinkingEnabled bool, newType *string, parts *[]ContentPart, path string) bool {
	switch val := v.(type) {
	case string:
		if val == "FINISHED" && (path == "" || path == "status") {
			return true
		}
		if isStatusPath(path) {
			return false
		}
		appendContentPart(parts, val, partType)
	case []any:
		pp, finished := extractContentRecursiveFrom(val, partType, thinkingEnabled, newType, path)
		if finished {
			return true
		}
		*parts = append(*parts, pp...)
	case map[string]any:
		if appendObjectContentByPath(path, val, partType, parts) {
			return false
		}
		appendWrappedFragments(val, partType, thinkingEnabled, newType, parts)
	}
	return false
}

func appendObjectContentByPath(path string, val map[string]any, partType string, parts *[]ContentPart) bool {
	if path != "response/content" && path != "response/thinking_content" && path != "" {
		return false
	}
	text, _ := val["text"].(string)
	if text == "" {
		text, _ = val["content"].(string)
	}
	if text == "" {
		return false
	}
	appendContentPart(parts, text, partType)
	return true
}

func appendWrappedFragments(val map[string]any, partType string, thinkingEnabled bool, newType *string, parts *[]ContentPart) {
	resp := val
	if wrapped, ok := val["response"].(map[string]any); ok {
		resp = wrapped
	}
	frags, ok := resp["fragments"].([]any)
	if !ok {
		return
	}
	for _, item := range frags {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typeName, content, fragType := parseFragmentTypeContent(m)
		if typeName == "" {
			typeName = fragType
		}
		switch typeName {
		case "THINK", "THINKING":
			if !bodyStarted(thinkingEnabled, *newType) {
				// A THINK fragment moves the sticky cursor to thinking: it is
				// the upstream's explicit "reasoning" marker, and RESPONSE (or
				// response/content) is the only way back to text. Once the body
				// has begun (one-size-fits-all rule) the cursor never rewinds;
				// the part is still appended so the detection layer sees the
				// upstream activity, then ParseSSEChunkForContentDetailed drops
				// it from visible parts.
				*newType = "thinking"
			}
			appendContentPart(parts, content, "thinking")
		case "RESPONSE":
			*newType = "text"
			appendContentPart(parts, content, "text")
		default:
			if isToolFragmentType(typeName) {
				appendContentPart(parts, content, "thinking")
			} else {
				appendContentPart(parts, content, partType)
			}
		}
	}
}

func parseFragmentTypeContent(m map[string]any) (string, string, string) {
	typeName, _ := m["type"].(string)
	content, _ := m["content"].(string)
	return strings.ToUpper(typeName), content, strings.ToUpper(typeName)
}

func appendContentPart(parts *[]ContentPart, content, kind string) {
	if content == "" {
		return
	}
	*parts = append(*parts, ContentPart{Text: content, Type: kind})
}
