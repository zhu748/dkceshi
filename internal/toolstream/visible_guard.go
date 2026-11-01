package toolstream

import (
	"strings"

	"ds2api/internal/sse"
	"ds2api/internal/toolcall"
)

// VisibleGuard removes tool protocol from a display-only channel. Raw input
// must still be retained separately for final detection, validation and repair.
// It never emits executable calls and keeps scanner state across SSE chunks.
type VisibleGuard struct {
	Prompt                string
	echo                  sse.PromptEchoGuard
	echoInit              bool
	Marker                string
	StripReferenceMarkers bool
	state                 State
}

func (g *VisibleGuard) Write(text string) string {
	if !g.echoInit {
		g.echo = sse.NewPromptEchoGuard(g.Prompt)
		g.echoInit = true
	}
	text = g.echo.Feed(text)
	g.state.DeferMalformed = true
	g.state.CleanVisible = true
	g.state.DisplayOnly = true
	g.state.Marker = g.Marker
	g.state.StripReferenceMarkers = g.StripReferenceMarkers
	return visibleGuardText(ProcessChunk(&g.state, text, nil))
}

func (g *VisibleGuard) Flush() string {
	tail := g.Write(g.echo.Flush())
	return tail + visibleGuardText(Flush(&g.state, nil))
}

func visibleGuardText(events []Event) string {
	var out strings.Builder
	for _, event := range events {
		if event.Deferred {
			idx, protocol := toolcall.FindToolProtocolPrefixOutsideIgnored(event.Content, 0)
			epse, canonical := toolcall.ContainsToolMarkupSyntaxOutsideIgnored(event.Content)
			if (idx >= 0 && protocol != "envelope_partial") || epse || canonical || toolcall.HasUnresolvedToolMarkup(event.Content) {
				continue
			}
		}
		out.WriteString(event.Content)
	}
	return out.String()
}

func FilterVisible(text, marker string, prompts ...string) string {
	guard := VisibleGuard{Marker: marker}
	if len(prompts) > 0 {
		guard.Prompt = prompts[0]
	}
	return guard.Write(text) + guard.Flush()
}
