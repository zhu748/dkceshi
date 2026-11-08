package files

import (
	"fmt"
	"mime"
	"strings"
)

// The encoding flag is a standalone header token. Semicolons inside quoted
// MIME parameters are literal, and MIME parameters must survive decoding.
func parseDataURLHeader(metadata, explicitContentType string) (string, bool, error) {
	var segments []string
	start := 0
	quoted, escaped := false, false
	for i := 0; i < len(metadata); i++ {
		ch := metadata[i]
		if escaped {
			escaped = false
			continue
		}
		if quoted && ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			quoted = !quoted
		}
		if ch == ';' && !quoted {
			segments = append(segments, metadata[start:i])
			start = i + 1
		}
	}
	segments = append(segments, metadata[start:])
	var media strings.Builder
	if first := strings.TrimSpace(segments[0]); first != "" {
		media.WriteString(first)
	} else {
		media.WriteString("application/octet-stream")
	}
	base64 := false
	for _, segment := range segments[1:] {
		if strings.EqualFold(strings.TrimSpace(segment), "base64") {
			if base64 {
				return "", false, fmt.Errorf("duplicate data URL encoding flag")
			}
			base64 = true
		} else {
			media.WriteByte(';')
			media.WriteString(segment)
		}
	}
	mediaType, params, err := mime.ParseMediaType(media.String())
	if err != nil {
		return "", false, fmt.Errorf("invalid data URL media type")
	}
	if explicit := strings.TrimSpace(explicitContentType); explicit != "" {
		return explicit, base64, nil
	}
	return mime.FormatMediaType(mediaType, params), base64, nil
}
