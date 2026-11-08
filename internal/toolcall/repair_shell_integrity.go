package toolcall

// An incomplete opening shell cannot disappear just because it has no complete
// name attribute yet. Complete named calls with missing closing tags remain
// repairable; their ordered identities are already known to the manifest.
func unfinishedRepairShellAt(text string, at int) bool {
	start, ok := consumeToolMarkupLessThan(text, at)
	if !ok {
		return false
	}
	if _, closing := consumeToolMarkupClosingSlash(text, start); closing {
		return false
	}
	name, size := matchToolMarkupName(text, start, false)
	return name != "" && hasToolMarkupBoundary(text, start+size)
}
