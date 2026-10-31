package sse

import (
	"regexp"
)

// bodyStarted reports whether visible body output has already begun for the
// current request: reasoning is enabled and the sticky segment cursor is
// already in the text segment. Once that happens the one-size-fits-all rule
// applies — any further think / thinking / reasoning increment is discarded.
func bodyStarted(thinkingEnabled bool, cursor string) bool {
	return thinkingEnabled && cursor == "text"
}

func updateTypeFromExplicitPath(path string, thinkingEnabled bool, newType *string) {
	if newType == nil {
		return
	}
	switch path {
	case "response/content":
		*newType = "text"
	case "response/thinking_content":
		if !thinkingEnabled || *newType != "text" {
			*newType = "thinking"
		}
	}
}

func selectThinkingParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		if p.Type == "thinking" {
			out = append(out, p)
		}
	}
	return out
}

func dropThinkingParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return parts
	}
	out := parts[:0]
	for _, p := range parts {
		if p.Type == "thinking" {
			continue
		}
		out = append(out, p)
	}
	return out
}

var thinkClosePattern = regexp.MustCompile(`(?i)</\s*think\s*>`)

var thinkOpenPattern = regexp.MustCompile(`(?i)<\s*think\s*>`)

// splitThinkingParts detects </think> inside thinking content and
// auto-transitions everything after it to text. This handles the
// DeepSeek API bug where the upstream SSE keeps sending
// reasoning_content even though the model has finished thinking.
func splitThinkingParts(parts []ContentPart) ([]ContentPart, bool) {
	var out []ContentPart
	thinkingDone := false
	for _, p := range parts {
		if thinkingDone && p.Type == "thinking" {
			// Already transitioned — treat remaining thinking as text.
			cleaned := stripThinkTags(p.Text)
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: "text"})
			}
			continue
		}
		if p.Type != "thinking" {
			cleaned := stripThinkTags(p.Text)
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: p.Type})
			}
			continue
		}
		loc := thinkClosePattern.FindStringIndex(p.Text)
		if loc == nil {
			out = append(out, p)
			continue
		}
		// Split at </think>: before is still thinking, after becomes text.
		thinkingDone = true
		before := p.Text[:loc[0]]
		after := p.Text[loc[1]:]
		if before != "" {
			out = append(out, ContentPart{Text: before, Type: "thinking"})
		}
		after = stripThinkTags(after)
		if after != "" {
			out = append(out, ContentPart{Text: after, Type: "text"})
		}
	}
	if !thinkingDone {
		// Return 'out' instead of 'parts' because text parts might have been cleaned via stripThinkTags
		return out, false
	}
	return out, true
}

// stripThinkTags removes any remaining <think> or </think> tags from text.
func stripThinkTags(s string) string {
	s = thinkClosePattern.ReplaceAllString(s, "")
	s = thinkOpenPattern.ReplaceAllString(s, "")
	return s
}
