package files

import (
	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
)

// Attachment bags are independent files, even when their owning block has just
// been uploaded. Keep them in the traversal model until all children succeed.
func (s *inlineUploadState) walkUploadedAttachments(replacement, source map[string]any) (map[string]any, error) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		if raw, present := source[key]; present {
			updated, err := s.walk(raw, true)
			if err != nil {
				return nil, err
			}
			promptcompat.AppendMessageAttachments(replacement, map[string]any{key: updated})
		}
	}
	for _, key := range uploadAttachmentContainers {
		if nested, ok := source[key].(map[string]any); ok {
			if _, err := s.walkUploadedAttachments(replacement, nested); err != nil {
				return nil, err
			}
		}
	}
	return replacement, nil
}

var uploadAttachmentContainers = []string{"file", "source", "inlineData", "inline_data", "image_url"}

// Do not scrub uploadable children into metadata before traversing them, or
// duplicate the same attachments in the visible file-reference description.
func uploadContextMetadata(source map[string]any) map[string]any {
	return prompt.AttachmentContextMetadata(withoutUploadAttachmentBags(source))
}

func withoutUploadAttachmentBags(source map[string]any) map[string]any {
	copy := make(map[string]any, len(source))
	for key, value := range source {
		if key != "attachments" && key != "files" {
			copy[key] = value
		}
	}
	for _, key := range uploadAttachmentContainers {
		if nested, ok := copy[key].(map[string]any); ok {
			copy[key] = withoutUploadAttachmentBags(nested)
		}
	}
	return copy
}
