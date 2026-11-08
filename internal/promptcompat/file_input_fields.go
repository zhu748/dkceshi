package promptcompat

// AttachmentInputKeys contains request-level attachment bodies, as opposed to
// ID-only lists. Upload, prompt conversion and continuation share this list.
func AttachmentInputKeys() []string { return []string{"attachments", "files"} }

func FileReferenceInputKeys() []string {
	return append([]string{"ref_file_ids", "file_ids"}, AttachmentInputKeys()...)
}

func InlineFileInputKeys() []string {
	return append([]string{"messages", "input", "contents", "system", "systemInstruction", "system_instruction"}, AttachmentInputKeys()...)
}
