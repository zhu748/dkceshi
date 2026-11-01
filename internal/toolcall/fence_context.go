package toolcall

// An unclosed Markdown fence is recoverable only when it began inside an
// already-open tool parameter. Closing tags inside an example are not proof
// that the example was an invocation.
func openToolParameterBeforeFence(text string) bool {
	depth := 0
	for offset := 0; offset < len(text); {
		tag, ok := FindToolMarkupTagOutsideIgnored(text, offset)
		if !ok {
			break
		}
		if tag.Name == "parameter" {
			if tag.Closing {
				depth = max(0, depth-1)
			} else {
				depth++
			}
		}
		offset = tag.End + 1
	}
	return depth > 0
}
