package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

// FileInputChildKeys is shared by upload and reference discovery. Tool JSON
// objects and function arguments are opaque data, not attachment containers.
func FileInputChildKeys(block map[string]any) []string {
	kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
	switch kind {
	case "text", "input_text", "output_text":
		return AttachmentInputKeys()
	case "thinking", "reasoning":
		return nil
	case "tool_use", "function_call", "custom_tool_call", "tool_call":
		return AttachmentInputKeys() // Arguments/input are opaque business data.
	case "document":
		if source, _ := block["source"].(map[string]any); source["type"] == "text" || source["type"] == "content" {
			return append(AttachmentInputKeys(), "source") // Text data is opaque; explicit attachment bags still apply.
		}
	}
	if IsInlineAttachmentBlock(block) {
		if id, err := prompt.ResolveFileReferenceID(block); err == nil && id != "" {
			return AttachmentInputKeys() // Old locator bytes must not become a second upload.
		}
	}
	role := strings.ToLower(strings.TrimSpace(asString(block["role"])))
	if role == "tool" || role == "function" || kind == "tool_result" || kind == "function_call_output" || kind == "custom_tool_call_output" {
		keys := AttachmentInputKeys()
		for _, key := range []string{"content", "output"} {
			switch value := block[key].(type) {
			case []any:
				keys = append(keys, key)
			case map[string]any:
				if IsInlineAttachmentBlock(value) {
					keys = append(keys, key)
				}
			}
		}
		return keys
	}
	return []string{"ref_file_ids", "file_ids", "attachments", "messages", "input", "content", "files", "items", "source", "file", "image_url", "contents", "parts", "functionCall", "functionResponse"}
}

// Untyped compatibility blocks must carry an explicit file/image payload key.
// A business record's name, filename, data or URL alone never declares a file.
func IsInlineAttachmentBlock(block map[string]any) bool {
	kind := strings.ToLower(strings.TrimSpace(asString(block["type"])))
	switch kind {
	case "file", "input_file", "file_reference", "image", "input_image", "image_url":
		return true
	case "document":
		source, _ := block["source"].(map[string]any)
		return source["type"] != "text" && source["type"] != "content"
	case "":
		for _, key := range []string{"file_data", "image_url", "inlineData", "inline_data"} {
			if block[key] != nil {
				return true
			}
		}
		if file, ok := block["file"].(map[string]any); ok {
			return IsInlineAttachmentBlock(file)
		}
	}
	return false
}
