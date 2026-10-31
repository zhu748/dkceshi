package toolcall

// An orphan CDATA close outside a value means a greedy boundary may have
// truncated a nested literal. A format-only repair cannot safely discard that
// tail. Reject this ambiguous interpretation so generation can retry instead.
func repairHasOrphanCDATA(text string) bool {
	for i := 0; i < len(text); {
		next, advanced, blocked := skipXMLIgnoredSection(text, i)
		if blocked {
			return false
		}
		if advanced {
			i = next
			continue
		}
		if end, ok := markdownCodeSpanEnd(text, i); ok {
			i = end
			continue
		}
		if toolCDATACloseLenAt(text, i) > 0 {
			return true
		}
		i++
	}
	return false
}
