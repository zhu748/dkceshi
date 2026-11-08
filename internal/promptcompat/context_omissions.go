package promptcompat

import (
	"sort"
	"strconv"
	"strings"

	"ds2api/internal/prompt"
)

// Omissions describe normalized fields without embedding their values. They
// expose missing serializers separately from intentional transport-only hints.
type ContextOmission struct {
	MessageIndex int    `json:"message_index"`
	Field        string `json:"field,omitempty"`
	Reason       string `json:"reason"`
}

func contextOmissions(messages []ContextMessageDigest) []ContextOmission {
	var out []ContextOmission
	for _, message := range messages {
		if message.OmittedReason != "" {
			out = append(out, ContextOmission{MessageIndex: message.Index, Reason: message.OmittedReason})
			continue
		}
		for _, field := range message.Fields {
			if field.OmittedReason != "" {
				out = append(out, ContextOmission{MessageIndex: message.Index, Field: field.Name, Reason: field.OmittedReason})
			}
		}
		for _, path := range message.OmittedPaths {
			out = append(out, ContextOmission{MessageIndex: message.Index, Field: path, Reason: "binary_or_opaque_payload"})
		}
	}
	return out
}

func contentOmittedPaths(message map[string]any) []string {
	var paths []string
	if strings.EqualFold(strings.TrimSpace(asString(message["role"])), "assistant") {
		for _, path := range prompt.AudioContextOmittedPaths(message["audio"]) {
			paths = append(paths, "/audio"+path)
		}
		calls, _ := message["tool_calls"].([]any)
		for index, raw := range calls {
			call, _ := raw.(map[string]any)
			for group, fields := range prompt.ToolCallContextFields(call) {
				base := "/tool_calls/" + strconv.Itoa(index)
				if group == "function" {
					base += "/function"
				}
				for _, path := range prompt.ContextValueOmittedPaths(fields) {
					paths = append(paths, base+path)
				}
			}
		}
	}
	for field, value := range prompt.MessageContextFields(message) {
		for _, path := range prompt.ContextValueOmittedPaths(value) {
			paths = append(paths, "/"+inputPathEscaper.Replace(field)+path)
		}
	}
	for _, field := range []string{"content", "reasoning_content", "attachments", "files"} {
		raw := message[field]
		wrapped := false
		if field == "attachments" || field == "files" {
			_, wrapped = raw.(map[string]any)
			raw = appendRequestAttachments(nil, raw)
		}
		if field == "reasoning_content" {
			if !strings.EqualFold(strings.TrimSpace(asString(message["role"])), "assistant") {
				continue
			}
			if block, ok := raw.(map[string]any); ok {
				raw = []any{block}
				wrapped = true
			}
		}
		for _, path := range prompt.ContentOmittedPaths(raw) {
			if wrapped {
				path = strings.TrimPrefix(path, "/0")
			}
			paths = append(paths, "/"+field+path)
		}
	}
	sort.Strings(paths)
	return paths
}
