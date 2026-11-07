package promptcompat

import (
	"fmt"
	"strings"
)

// These block shapes have no upload/decoding adapter. Reject them before the
// binary scrubber can turn the only input into a placeholder and continue.
// Opaque business JSON is never passed here as a content-block array.
func ValidateMediaContentBlock(block map[string]any) error {
	kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
	switch kind {
	case "input_audio", "input_video", "audio", "video":
		return fmt.Errorf("content type %q is not supported; provide a text transcript or a supported file attachment", kind)
	}
	return validateFileSource(block, kind)
}
