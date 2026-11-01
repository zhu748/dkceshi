package textclean

func CleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	if text == "" {
		return text
	}
	if stripReferenceMarkers {
		text = StripReferenceMarkers(text)
	}
	var cleaner StreamCleaner
	return cleaner.Write(text) + cleaner.Flush()
}

// CleanVisibleDelta only applies reference policy to text already classified
// by the stateful stream guard. Re-running markup cleanup on isolated deltas
// would lose the enclosing Markdown/JSON literal context.
func CleanVisibleDelta(text string, stripReferenceMarkers bool) string {
	if stripReferenceMarkers {
		return StripReferenceMarkers(text)
	}
	return text
}
