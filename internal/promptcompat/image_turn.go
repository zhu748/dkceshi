package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

// Uploaded untyped image_url attachments use the same reference serializer as
// typed images. Keep their labels and metadata instead of losing the attachment.
func imageReferenceText(block map[string]any) string {
	if strings.TrimSpace(asString(block["type"])) == "" && isImageContentBlock(block) {
		block = shallowCopyMap(block)
		block["type"] = "image_url"
	}
	return prompt.FileReferenceText(block)
}
