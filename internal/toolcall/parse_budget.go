package toolcall

import "ds2api/internal/streambudget"

// Bound local work before recursive parsing, integrity probes or LLM repair.
// Count source nodes, including damaged calls, rather than only parsed calls.
func toolMarkupLimitExceeded(text string) bool {
	depth, nodes, calls := 0, 0, 0
	maxCalls := streambudget.Value("max_tool_calls")
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
		tag, ok := scanToolMarkupTagAt(text, i)
		if !ok {
			if native, found := scanDSMLTagAt(text, i); found {
				tag, ok = ToolMarkupTag{Name: native.Name, End: native.End, Closing: native.Closing}, true
			}
		}
		if !ok {
			i = nextToolMarkupProbe(text, i)
			continue
		}
		i = tag.End + 1
		nodes++
		if tag.Closing {
			if depth > 0 {
				depth--
			}
		} else if !tag.SelfClosing {
			depth++
			if tag.Name == "invoke" {
				calls++
			}
		}
		if depth > 128 || nodes > 32768 || calls > maxCalls {
			return true
		}
	}
	return false
}
