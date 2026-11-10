package toolcall

import (
	"encoding/xml"
	"strings"
)

// XML normalizes literal CR/CRLF before returning CharData. Escape only source
// carriage returns in this already validated text token; entities still decode
// exactly once, including numeric references and escaped entity examples.
func decodeXMLTextWithLineEndings(raw string) (string, error) {
	var value struct {
		Text string `xml:",chardata"`
	}
	err := xml.Unmarshal([]byte("<text>"+strings.ReplaceAll(raw, "\r", "&#13;")+"</text>"), &value)
	return value.Text, err
}
