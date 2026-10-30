package chathistory

import (
	"encoding/json"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/prompt"
)

// SnapshotJSON preserves structured diagnostics independently of the request's
// mutable maps, including json.Number values that must not pass through float64.
func SnapshotJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		config.Logger.Warn("[chat_history] structured snapshot failed", "error", err)
		return nil
	}
	return data
}

// CaptureMessages keeps tool-only assistant turns and empty/error tool results.
// Content stays compatible with the history UI; structured fields make the
// request's call/result pairs available to offline diagnostics as well.
func CaptureMessages(messages []any) []Message {
	out := make([]Message, 0, len(messages))
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		name, _ := msg["name"].(string)
		id, _ := msg["id"].(string)
		callID, _ := msg["tool_call_id"].(string)
		item := Message{
			Role: role, Content: prompt.NormalizeContent(msg["content"]),
			Name: name, ID: id, ToolCallID: callID,
			ToolCalls: SnapshotJSON(msg["tool_calls"]), FunctionCall: SnapshotJSON(msg["function_call"]),
			ReasoningContent: SnapshotJSON(msg["reasoning_content"]),
		}
		if flag, exists := msg["is_error"].(bool); exists {
			item.IsError = &flag
		}
		if flag, exists := msg["prefix"].(bool); exists {
			item.Prefix = &flag
		}
		out = append(out, item)
	}
	return out
}
