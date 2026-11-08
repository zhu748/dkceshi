package responses

import (
	"encoding/json"

	"ds2api/internal/jsonvalue"
	"ds2api/internal/promptcompat"
)

type replayFileInputs map[string]any

const replayFilesKey = "_responses_replay_files"

type replayContext struct {
	Messages []any          `json:"messages"`
	Files    map[string]any `json:"files,omitempty"`
}

func marshalReplayContext(messages []any, fileInputs ...map[string]any) ([]byte, error) {
	if len(fileInputs) > 0 && len(fileInputs[0]) > 0 {
		return json.Marshal(replayContext{Messages: messages, Files: fileInputs[0]})
	}
	return json.Marshal(messages)
}

func unmarshalReplayContext(raw []byte) ([]any, map[string]any, bool) {
	if len(raw) > 0 && raw[0] == '{' {
		var replay replayContext
		if err := jsonvalue.Unmarshal(raw, &replay); err != nil {
			return nil, nil, false
		}
		return replay.Messages, replay.Files, true
	}
	var messages []any
	if err := jsonvalue.Unmarshal(raw, &messages); err != nil {
		return nil, nil, false
	}
	return messages, nil, true
}

// Capture caller inputs before upload preprocessing rewrites inline attachments.
// Keeping the original bytes also preserves the existing account-pool replay.
func captureReplayFiles(req map[string]any) replayFileInputs {
	files := make(replayFileInputs)
	for _, key := range promptcompat.FileReferenceInputKeys() {
		if value := req[key]; value != nil {
			files[key] = cloneValue(value)
		}
	}
	return files
}

// Both maps are request-owned (stored context is decoded into fresh values).
// Keep raw bytes in their original message for subsequent uploads/account
// replay; only ID-only lists remain global. This also migrates legacy bags.
func scopeReplayAttachments(messages []any, inputs map[string]any) []any {
	if len(inputs) == 0 {
		return messages
	}
	messages, _ = promptcompat.WithRequestAttachmentContext(messages, inputs)
	for _, key := range promptcompat.AttachmentInputKeys() {
		delete(inputs, key)
	}
	return messages
}

func mergeReplayFiles(req map[string]any, previous map[string]any) {
	for _, key := range promptcompat.FileReferenceInputKeys() {
		if old := previous[key]; old != nil {
			var merged []any
			merged = appendReplayInputs(merged, old)
			merged = appendReplayInputs(merged, req[key])
			req[key] = merged
		}
		if (key == "ref_file_ids" || key == "file_ids") && req[key] != nil {
			req[key] = promptcompat.CollectOpenAIRefFileIDs(map[string]any{key: req[key]})
		}
	}
	// Never trust a caller-supplied snapshot. Capture only after authentication.
	delete(req, replayFilesKey)
}

func appendReplayInputs(out []any, raw any) []any {
	switch items := raw.(type) {
	case []any:
		for _, item := range items {
			out = appendReplayInputs(out, item)
		}
	case []string:
		for _, item := range items {
			out = append(out, item)
		}
	case nil:
	default:
		out = append(out, raw)
	}
	return out
}
