package toolcall

import "strings"

// Canonical XML is accepted by the parser, so an unparsed canonical sibling
// must also reach repair/validation. Otherwise a valid call can hide the rest
// of a malformed batch. Orphan closing shells and code examples are excluded.
func hasCanonicalToolInvocation(text string) bool {
	for from := 0; from < len(text); {
		tag, ok := FindToolMarkupTagOutsideIgnored(text, from)
		if !ok {
			return false
		}
		if tag.Canonical && !tag.Closing && tag.Name == "invoke" {
			attrs := parseXMLTagAttributes(text[tag.NameEnd:tag.End])
			if strings.TrimSpace(attrs["name"]) != "" {
				return true
			}
		}
		from = tag.End + 1
	}
	return false
}
