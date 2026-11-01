package sse

import (
	"context"
	"strings"
)

// Repeated APPEND/delta text is real data, including identical tool calls.
// Deduplication is reserved for explicit SETs and replay snapshot envelopes.
func isContentSnapshot(chunk map[string]any) bool {
	op, _ := chunk["o"].(string)
	if strings.EqualFold(op, "SET") {
		return true
	}
	if strings.EqualFold(op, "APPEND") {
		return false
	}
	value, ok := chunk["v"].(map[string]any)
	if !ok {
		return false
	}
	if response, ok := value["response"].(map[string]any); ok {
		value = response
	}
	_, ok = value["fragments"].([]any)
	return ok
}

func ContentPartDelta(existing *strings.Builder, part ContentPart) string {
	if part.Snapshot && existing != nil {
		return ExplicitSnapshotDelta(existing.String(), part.Text)
	}
	return part.Text
}

// Snapshot provenance is explicit, so even short replayed fields can overlap.
// The heuristic helper remains available for callers that only have text.
func ExplicitSnapshotDelta(existing, incoming string) string {
	if incoming == "" || existing == "" {
		return incoming
	}
	if strings.HasPrefix(incoming, existing) {
		return incoming[len(existing):]
	}
	if strings.HasPrefix(existing, incoming) {
		return ""
	}
	return incoming
}

func hasSnapshotParts(result LineResult) bool {
	for _, parts := range [][]ContentPart{result.Parts, result.ToolDetectionThinkingParts} {
		for _, part := range parts {
			if part.Snapshot {
				return true
			}
		}
	}
	return false
}

func forwardSnapshot(ctx context.Context, out chan<- LineResult, result LineResult, flush func(bool)) error {
	flush(true)
	data := result
	data.Stop = false
	frames := []LineResult{data}
	if result.Stop {
		result.Parts, result.ToolDetectionThinkingParts = nil, nil
		frames = append(frames, result)
	}
	for _, frame := range frames {
		select {
		case out <- frame:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
