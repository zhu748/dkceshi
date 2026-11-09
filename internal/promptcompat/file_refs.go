package promptcompat

import (
	"strings"

	"ds2api/internal/prompt"
)

func CollectOpenAIRefFileIDs(req map[string]any) []string {
	if len(req) == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	for _, key := range append(FileReferenceInputKeys(), "messages", "input", "contents", "system", "systemInstruction", "system_instruction") {
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
		attachment := IsInlineAttachmentBlock(x) || kind == "document"
		if attachment || allowStrings {
			block := x
			if allowStrings {
				block = NormalizeAttachmentInputBlock(x)
			}
			if fileID, err := prompt.ResolveFileReferenceID(block); err == nil {
				addOpenAIRefFileID(out, seen, fileID)
			}
		}
		// Bare strings count only inside explicit ID lists. Text arrays and
		// tool-result data remain conversation content at every nesting level.
		for _, key := range FileInputChildKeys(x) {
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
