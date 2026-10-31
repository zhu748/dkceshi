package textclean

func CleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	if text == "" {
		return text
	}
	if stripReferenceMarkers {
		text = StripReferenceMarkers(text)
	}
	return sanitizeLeakedOutput(text)
}
