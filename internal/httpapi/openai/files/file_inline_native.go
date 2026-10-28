package files

import (
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/httpapi/openai/shared"
)

// Native formats are translated to the same decoded file shape as OpenAI.
// No remote URL is fetched and no base64 content is left in the visible prompt.
func decodeNativeInlineBlock(block map[string]any, kind string) (inlineDecodedFile, bool, error) {
	var source map[string]any
	if kind == "image" || kind == "document" {
		source, _ = block["source"].(map[string]any)
		if source == nil || source["type"] != "base64" {
			return inlineDecodedFile{}, false, nil
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
	return inlineDecodedFile{Data: data, ContentType: mime, Filename: pickInlineFilename(block, mime, prefix), ReplacementType: replacement}, true, nil
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
