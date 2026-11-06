package toolcall

// A failed probe at a run of opening delimiters also rules out every suffix
// of that run. Revisit its last delimiter so CDATA/comments beginning there
// still establish a literal boundary before scanning their contents.
func nextToolMarkupProbe(text string, start int) int {
	if text[start] < 0x80 && text[start] != '<' {
		return start + 1
	}
	last := start
	next, ok := consumeToolMarkupLessThan(text, start)
	for ok {
		following, found := consumeToolMarkupLessThan(text, next)
		if !found {
			break
		}
		last, next = next, following
	}
	if last > start {
		return last
	}
	return start + 1
}
