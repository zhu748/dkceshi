package assistantturn

import (
	"ds2api/internal/textclean"
	"ds2api/internal/toolcall"
	"ds2api/internal/toolstream"
)

// The original source locates the executable boundary even after malformed
// markup recovery. Visible text alone has already lost those source positions.
func toolTextForTurn(source, visible string, calls []toolcall.ParsedToolCall, opts BuildOptions, links map[int]string) string {
	if len(calls) == 0 {
		return toolstream.FilterVisible(visible, opts.ToolMarker)
	}
	prefix := toolstream.TextBeforeToolBoundary(source)
	text := textclean.CleanVisibleOutput(toolstream.FilterVisible(prefix, opts.ToolMarker), opts.StripReferenceMarkers)
	if opts.SearchEnabled {
		text = textclean.ReplaceCitationMarkersWithLinks(text, links)
	}
	return text
}
