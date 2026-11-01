package sse

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

// splitThinkingParts detects </think> inside thinking content and
// auto-transitions everything after it to text. This handles the
// DeepSeek API bug where the upstream SSE keeps sending
// reasoning_content even though the model has finished thinking.
func splitThinkingParts(parts []ContentPart, boundaries ...*ThinkingBoundary) ([]ContentPart, bool) {
	boundary := &ThinkingBoundary{}
	if len(boundaries) > 0 && boundaries[0] != nil {
		boundary = boundaries[0]
	}
	var out []ContentPart
	thinkingDone := false
	for _, p := range parts {
		if thinkingDone && p.Type == "thinking" {
			// Already transitioned — treat remaining thinking as text.
			cleaned := p.Text
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: "text", Snapshot: p.Snapshot})
			}
			continue
		}
		if p.Type != "thinking" {
			if tail := boundary.flush(); tail != "" {
				out = append(out, ContentPart{Text: tail, Type: "thinking"})
			}
			cleaned := p.Text
			if cleaned != "" {
				out = append(out, ContentPart{Text: cleaned, Type: p.Type, Snapshot: p.Snapshot})
			}
			continue
		}
		before, after, closed := boundary.split(p.Text)
		if !closed {
			if before != "" {
				p.Text = before
				out = append(out, p)
			}
			continue
		}
		thinkingDone = true
		if before != "" {
			out = append(out, ContentPart{Text: before, Type: "thinking", Snapshot: p.Snapshot})
		}
		if after != "" {
			out = append(out, ContentPart{Text: after, Type: "text", Snapshot: p.Snapshot})
		}
	}
	if !thinkingDone {
		// Keep payload text intact; only a confirmed thinking boundary is consumed.
		return out, false
	}
	return out, true
}
