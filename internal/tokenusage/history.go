package tokenusage

import (
	"context"
	"errors"
	"strings"
)

// Resolver is provided by the upstream response body (or a Vercel lease).
// It is called at successful completion, never for each output delta.
type Resolver interface {
	ResolveUpstreamUsage(context.Context, int) (Observation, error)
}

// FromHistory pairs the exact assistant message with its USER parent. The
// latest Android HAR reports the prompt-side cumulative count on USER and
// prompt+completion on ASSISTANT. Never substitute a different/latest message.
func FromHistory(payload map[string]any, sessionID string, messageID int) (Observation, error) {
	bad := errors.New("upstream message usage is unavailable or inconsistent")
	data, _ := payload["data"].(map[string]any)
	biz, _ := data["biz_data"].(map[string]any)
	if session, ok := biz["chat_session"].(map[string]any); ok {
		if id, _ := session["id"].(string); id != "" && id != sessionID {
			return Observation{}, bad
		}
	}
	items, _ := biz["chat_messages"].([]any)
	var assistant map[string]any
	for _, item := range items {
		m, _ := item.(map[string]any)
		if id, ok := Count(m["message_id"]); ok && id == messageID && messageID > 0 {
			assistant = m
			break
		}
	}
	if !historyRole(assistant, "ASSISTANT") {
		return Observation{}, bad
	}
	parent, ok := Count(assistant["parent_id"])
	if !ok || parent <= 0 {
		return Observation{}, bad
	}
	var user map[string]any
	seen := map[int]bool{messageID: true}
	for depth := 0; depth < MaxMessages && parent > 0 && !seen[parent]; depth++ {
		seen[parent] = true
		var node map[string]any
		for _, item := range items {
			m, _ := item.(map[string]any)
			if id, ok := Count(m["message_id"]); ok && id == parent {
				node = m
				break
			}
		}
		role, _ := node["role"].(string)
		if strings.EqualFold(role, "USER") {
			user = node
			break
		}
		if !strings.EqualFold(role, "ASSISTANT") {
			break
		}
		parent, _ = Count(node["parent_id"])
	}
	if !historyRole(user, "USER") {
		return Observation{}, bad
	}
	input, inputOK := Count(user["accumulated_token_usage"])
	total, totalOK := Count(assistant["accumulated_token_usage"])
	if !inputOK || !totalOK || total < input {
		return Observation{}, bad
	}
	output := total - input
	return Observation{MessageID: messageID, InputTokens: &input, OutputTokens: &output, TotalTokens: &total, AccumulatedTokens: &total}, nil
}

func historyRole(m map[string]any, role string) bool {
	got, _ := m["role"].(string)
	status, _ := m["status"].(string)
	return strings.EqualFold(got, role) && strings.EqualFold(status, "FINISHED")
}
