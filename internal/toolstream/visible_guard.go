package toolstream

import (
	"strings"

	"ds2api/internal/toolcall"
)

// VisibleGuard removes tool protocol from a display-only channel. Raw input
// must still be retained separately for final detection, validation and repair.
// It never emits executable calls and keeps scanner state across SSE chunks.
type VisibleGuard struct {
	Marker string
	state  State
}

func (g *VisibleGuard) Write(text string) string {
	g.state.DeferMalformed = true
	g.state.DisplayOnly = true
	g.state.Marker = g.Marker
	return visibleGuardText(ProcessChunk(&g.state, text, nil))
}

func (g *VisibleGuard) Flush() string {
	return visibleGuardText(Flush(&g.state, nil))
}

func visibleGuardText(events []Event) string {
	var out strings.Builder
	for _, event := range events {
		if event.Deferred {
			idx, _ := toolcall.FindToolProtocolPrefixOutsideIgnored(event.Content, 0)
			epse, canonical := toolcall.ContainsToolMarkupSyntaxOutsideIgnored(event.Content)
			if idx >= 0 || epse || canonical || toolcall.HasUnresolvedToolMarkup(event.Content) {
				continue
			}
		}
		out.WriteString(event.Content)
	}
	return out.String()
}

func FilterVisible(text, marker string) string {
	if !strings.ContainsAny(text, "<＜﹤〈") {
		return text
	}
	guard := VisibleGuard{Marker: marker}
	return guard.Write(text) + guard.Flush()
}
