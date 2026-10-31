package sse

import (
	"bytes"
	"encoding/json"
	"strings"
)

type ContentPart struct {
	Text string
	Type string
}

var dataPrefix = []byte("data:")

var donePayload = []byte("[DONE]")

// ParseDeepSeekSSELine stays on []byte end to end: the previous
// string(raw) -> []byte(dataStr) round trip copied every SSE line twice on the
// hottest path in the process (once per line in both the pump and collect
// consumers).
func ParseDeepSeekSSELine(raw []byte) (map[string]any, bool, bool) {
	line := bytes.TrimSpace(raw)
	if len(line) == 0 || !bytes.HasPrefix(line, dataPrefix) {
		return nil, false, false
	}
	dataBytes := bytes.TrimSpace(bytes.TrimPrefix(line, dataPrefix))
	if bytes.Equal(dataBytes, donePayload) {
		return nil, true, true
	}
	chunk := map[string]any{}
	if err := json.Unmarshal(dataBytes, &chunk); err != nil {
		return nil, false, false
	}
	return chunk, false, true
}

func ParseSSEChunkForContent(chunk map[string]any, thinkingEnabled bool, currentFragmentType string) ([]ContentPart, bool, string) {
	parts, _, finished, nextType := ParseSSEChunkForContentDetailed(chunk, thinkingEnabled, currentFragmentType)
	return parts, finished, nextType
}

func ParseSSEChunkForContentDetailed(chunk map[string]any, thinkingEnabled bool, currentFragmentType string) ([]ContentPart, []ContentPart, bool, string) {
	v, ok := chunk["v"]
	if !ok {
		return nil, nil, false, currentFragmentType
	}
	path, _ := chunk["p"].(string)
	if shouldSkipPath(path) {
		return nil, nil, false, currentFragmentType
	}
	if isStatusPath(path) {
		if s, ok := v.(string); ok {
			if strings.EqualFold(strings.TrimSpace(s), "FINISHED") {
				return nil, nil, true, currentFragmentType
			}
			return nil, nil, false, currentFragmentType
		}
	}
	newType := currentFragmentType
	parts := make([]ContentPart, 0, 8)
	// The one-size-fits-all rule keys off whether visible body output had
	// already begun when this chunk arrived (the sticky cursor was already in
	// the text segment). Once the body has begun, every residual thinking /
	// reasoning increment in this and every following chunk is dropped
	// wholesale: it is neither routed to thinking (no reasoning_content
	// increments) nor recovered into the body (no thinking content folded into
	// content). The </think> text-recovery in splitThinkingParts only applies
	// while the message is still in its reasoning phase.
	bodyStartedAtStart := bodyStarted(thinkingEnabled, currentFragmentType)
	updateTypeFromExplicitPath(path, thinkingEnabled, &newType)
	collectDirectFragments(path, chunk, v, thinkingEnabled, &newType, &parts)
	updateTypeFromNestedResponse(path, v, thinkingEnabled, &newType)
	partType := resolvePartType(path, thinkingEnabled, newType)
	finished := appendChunkValueContent(v, partType, thinkingEnabled, &newType, &parts, path)
	if finished {
		return nil, nil, true, newType
	}
	if bodyStartedAtStart {
		// The upstream THINK parts are still surfaced to the caller's
		// detection layer (ToolDetectionThinkingParts) so upstream activity is
		// not mistaken for an idle/empty stream, but they never reach visible
		// Parts.
		detection := selectThinkingParts(parts)
		cleaned := parts[:0]
		for _, p := range parts {
			if p.Type == "thinking" {
				continue
			}
			p.Text = stripThinkTags(p.Text)
			if p.Text != "" {
				cleaned = append(cleaned, p)
			}
		}
		return cleaned, detection, false, newType
	}
	var transitioned bool
	parts, transitioned = splitThinkingParts(parts)
	if transitioned {
		newType = "text"
	}
	detectionThinkingParts := selectThinkingParts(parts)
	if !thinkingEnabled {
		parts = dropThinkingParts(parts)
	}
	return parts, detectionThinkingParts, false, newType
}
