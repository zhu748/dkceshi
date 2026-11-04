package promptcompat

import "strings"

func CollectOpenAIRefFileIDs(req map[string]any) []string {
	if len(req) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, key := range []string{
		"ref_file_ids",
		"file_ids",
		"attachments",
		"files",
		"messages",
		"input",
		"contents",
	} {
		raw := req[key]
		if raw == nil {
			continue
		}
		appendOpenAIRefFileIDs(&out, seen, raw, referenceIDContainer(key))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func referenceIDContainer(key string) bool {
	return key == "ref_file_ids" || key == "file_ids" || key == "attachments" || key == "files"
}

func appendOpenAIRefFileIDs(out *[]string, seen map[string]struct{}, raw any, allowStrings bool) {
	switch x := raw.(type) {
	case string:
		if allowStrings {
			addOpenAIRefFileID(out, seen, x)
		}
	case []string:
		if allowStrings {
			for _, item := range x {
				addOpenAIRefFileID(out, seen, item)
			}
		}
	case []any:
		for _, item := range x {
			appendOpenAIRefFileIDs(out, seen, item, allowStrings)
		}
	case map[string]any:
		kind := strings.ToLower(strings.TrimSpace(asString(x["type"])))
		attachment := kind == "file" || kind == "input_file" || kind == "file_reference" || kind == "image" || kind == "input_image" || kind == "image_url" || kind == "document"
		if fileID := strings.TrimSpace(asString(x["file_id"])); fileID != "" && (attachment || allowStrings) {
			addOpenAIRefFileID(out, seen, fileID)
		}
		if kind == "file" || kind == "input_file" || kind == "file_reference" {
			if fileID := strings.TrimSpace(asString(x["id"])); fileID != "" {
				addOpenAIRefFileID(out, seen, fileID)
			}
		}
		if fileMap, ok := x["file"].(map[string]any); ok && (attachment || allowStrings) {
			if fileID := strings.TrimSpace(asString(fileMap["file_id"])); fileID != "" {
				addOpenAIRefFileID(out, seen, fileID)
			}
			if fileID := strings.TrimSpace(asString(fileMap["id"])); fileID != "" {
				addOpenAIRefFileID(out, seen, fileID)
			}
		}
		// Bare strings count only inside explicit ID lists. Text arrays and
		// tool-result data remain conversation content at every nesting level.
		for _, key := range []string{"ref_file_ids", "file_ids", "attachments", "messages", "input", "content", "files", "items", "data", "source", "contents", "parts"} {
			if nested, ok := x[key]; ok {
				appendOpenAIRefFileIDs(out, seen, nested, referenceIDContainer(key))
			}
		}
	}
}

func addOpenAIRefFileID(out *[]string, seen map[string]struct{}, fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return
	}
	if _, ok := seen[fileID]; ok {
		return
	}
	seen[fileID] = struct{}{}
	*out = append(*out, fileID)
}
