package sse

import (
	"strings"

	"ds2api/internal/toolcall"
)

// ThinkingBoundary keeps literal, partial delimiter and replay state between
// SSE frames. Only confirmed structural delimiters change the output channel.
type ThinkingBoundary struct {
	literal toolcall.LiteralState
	pending string
	replay  snapshotDecoder
}

const maxThinkingDelimiter = 256

func thinkingClosePrefix(text string) (bool, bool) {
	if len(text) < 2 {
		return text == "<", false
	}
	if !strings.HasPrefix(text, "</") {
		return false, false
	}
	rest := strings.TrimLeft(text[2:], " \t\r\n\f")
	if len(rest) <= 5 {
		return strings.HasPrefix("think", strings.ToLower(rest)), false
	}
	if !strings.EqualFold(rest[:5], "think") {
		return false, false
	}
	rest = strings.TrimLeft(rest[5:], " \t\r\n\f")
	return rest == "" || rest == ">", rest == ">"
}

func (b *ThinkingBoundary) split(text string) (string, string, bool) {
	var before strings.Builder
	for i, r := range text {
		structural := b.literal.Feed(r)
		if b.pending != "" {
			candidate := b.pending + string(r)
			prefix, complete := thinkingClosePrefix(candidate)
			if complete {
				b.pending = ""
				return before.String(), text[i+len(string(r)):], true
			}
			if prefix && len(candidate) <= maxThinkingDelimiter {
				b.pending = candidate
				continue
			}
			before.WriteString(b.pending)
			b.pending = ""
		}
		if structural && r == '<' {
			b.pending = "<"
		} else {
			before.WriteRune(r)
		}
	}
	return before.String(), "", false
}

func (b *ThinkingBoundary) flush() string {
	text := b.pending
	b.pending = ""
	return text
}

func (b *ThinkingBoundary) pendingLine(thinkingEnabled bool, currentType string) LineResult {
	result := LineResult{Parsed: true, NextType: currentType}
	if text := b.flush(); text != "" {
		part := ContentPart{Text: text, Type: "thinking"}
		result.ToolDetectionThinkingParts = []ContentPart{part}
		if thinkingEnabled {
			result.Parts = []ContentPart{part}
		}
	}
	return result
}

// Successful terminal statuses can arrive without [DONE]. Release a partial
// literal before the terminal frame, while consumers still accept content.
func finishThinkingLine(result LineResult, thinkingEnabled bool, boundaries ...*ThinkingBoundary) LineResult {
	if result.Stop && !result.ContentFilter && result.ErrorMessage == "" && len(boundaries) > 0 && boundaries[0] != nil {
		tail := boundaries[0].pendingLine(thinkingEnabled, result.NextType)
		result.Parts = append(result.Parts, tail.Parts...)
		result.ToolDetectionThinkingParts = append(result.ToolDetectionThinkingParts, tail.ToolDetectionThinkingParts...)
	}
	return result
}
