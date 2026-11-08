package files

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"ds2api/internal/httpapi/openai/shared"
)

func explicitInlineFilename(block map[string]any) string {
	for _, key := range []string{"filename", "file_name", "name"} {
		if name := strings.TrimSpace(shared.AsString(block[key])); name != "" {
			return filepath.Base(name)
		}
	}
	return ""
}

func decodeNestedInlineFileBlock(block, nested map[string]any) (inlineDecodedFile, bool, error) {
	if nested["type"] == nil {
		kind := strings.ToLower(strings.TrimSpace(shared.AsString(block["type"])))
		if kind == "file" || kind == "input_file" || kind == "file_reference" {
			copy := make(map[string]any, len(nested)+1)
			for key, value := range nested {
				copy[key] = value
			}
			copy["type"] = kind
			nested = copy
		}
	}
	decoded, matched, err := decodeOpenAIInlineFileBlock(nested)
	if err != nil || !matched {
		return decoded, matched, err
	}
	if explicitInlineFilename(nested) == "" {
		if name := explicitInlineFilename(block); name != "" {
			decoded.Filename = name
		}
	}
	if decoded.DisplayName == "" {
		decoded.DisplayName = inlineDisplayName(block)
	}
	return decoded, true, nil
}

// Native formats are translated to the same decoded file shape as OpenAI.
// No remote URL is fetched and no base64 content is left in the visible prompt.
func decodeNativeInlineBlock(block map[string]any, kind string) (inlineDecodedFile, bool, error) {
	var source map[string]any
	if kind == "image" || kind == "document" {
		raw, present := block["source"]
		if !present {
			return inlineDecodedFile{}, false, nil
		}
		var ok bool
		source, ok = raw.(map[string]any)
		if !ok {
			return inlineDecodedFile{}, true, fmt.Errorf("invalid native file source")
		}
		if source["type"] == "url" && kind == "image" {
			return inlineDecodedFile{}, false, nil // Shared image locator decoding follows.
		}
		if source["type"] != "base64" {
			return inlineDecodedFile{}, true, fmt.Errorf("unsupported native file source type")
		}
	}
	for _, key := range []string{"inlineData", "inline_data"} {
		if value, ok := block[key].(map[string]any); ok {
			source = value
			break
		}
	}
	if source == nil {
		return inlineDecodedFile{}, false, nil
	}
	mime := contentTypeFromMap(source)
	data, mime, err := decodeInlinePayload(shared.AsString(source["data"]), mime)
	if err != nil {
		return inlineDecodedFile{}, true, fmt.Errorf("invalid inline file input")
	}
	replacement, prefix := "input_file", "file"
	if kind == "image" || strings.HasPrefix(strings.ToLower(mime), "image/") {
		replacement, prefix = "input_image", "image"
	}
	labels := block
	if explicitInlineFilename(block) == "" && explicitInlineFilename(source) != "" {
		labels = source
	}
	display := inlineDisplayName(source)
	if display == "" {
		display = inlineDisplayName(block)
	}
	return inlineDecodedFile{Data: data, ContentType: mime, Filename: pickInlineFilename(labels, mime, prefix), ReplacementType: replacement, DisplayName: display}, true, nil
}

func inlineDisplayName(source map[string]any) string {
	for _, key := range []string{"displayName", "display_name"} {
		if name, ok := source[key].(string); ok && name != "" {
			return name
		}
	}
	return ""
}

func InlineFileErrorDetails(err error) (int, string) {
	inline, ok := err.(*inlineFileUploadError)
	if !ok || inline == nil {
		return http.StatusInternalServerError, "Failed to process file input."
	}
	status := inline.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	message := strings.TrimSpace(inline.message)
	if message == "" {
		message = "Failed to process file input."
	}
	return status, message
}
