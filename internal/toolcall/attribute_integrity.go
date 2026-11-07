package toolcall

// Unknown attributes may carry argument values. Reject the whole source batch
// before canonicalization or repair can discard them. Consume quoted values as
// units so text such as name="other" inside an attribute cannot create a call.
func unsupportedToolTagAttributes(text string, tag ToolMarkupTag) bool {
	if native, ok := scanDSMLTagAt(text, tag.Start); ok && (native.Name == "calls" || native.Name == "invoke" || native.Name == "parameter") {
		attrs, valid := dsmlRecoveryAttrs(text, native)
		if !valid {
			return true
		}
		for key := range attrs {
			if native.Closing || native.Name == "calls" || key != "name" && (native.Name != "parameter" || key != "string") {
				return true
			}
		}
		return false
	}
	canonical := canonicalizeRecognizedToolMarkupTag(text[tag.Start:tag.End+1], tag)
	attrs := parseXMLTagAttributes(canonical)
	if attrs == nil {
		return true
	}
	for key := range attrs {
		allowed := !tag.Closing && (tag.Name == "invoke" && (key == "name" || key == "call_id") || tag.Name == "parameter" && key == "name" || tag.Name == "tool_calls" && (key == "id" || key == "version"))
		if !allowed {
			return true
		}
	}
	return false
}
