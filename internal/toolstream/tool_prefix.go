package toolstream

// TextBeforeToolBoundary is used only after final validation accepted calls.
// It shares the stream's literal-aware interception rules, including DSML and
// malformed branded shells recovered at finalization. No boundary means there
// is no safe body prefix (for example a call recovered from hidden reasoning).
func TextBeforeToolBoundary(text string) string {
	state := State{DeferMalformed: true}
	if start := findToolSegmentStart(&state, text); start >= 0 {
		return text[:start]
	}
	return ""
}

// ToolResponseText also accepts an already-cleaned prefix from a validated
// turn. It must not erase that prefix simply because its tags were removed.
func ToolResponseText(text string) string {
	state := State{DeferMalformed: true}
	if start := findToolSegmentStart(&state, text); start >= 0 {
		text = text[:start]
	}
	return FilterVisible(text, "")
}
