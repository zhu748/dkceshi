package gemini

import (
	"strings"

	"ds2api/internal/assistantturn"
	"github.com/google/uuid"
)

func buildGeminiPartsFromTurn(turn assistantturn.Turn) []map[string]any {
	thinkingPart := func() []map[string]any {
		if turn.Thinking == "" {
			return nil
		}
		return []map[string]any{{"text": turn.Thinking, "thought": true}}
	}
	if len(turn.ToolCalls) > 0 {
		parts := thinkingPart()
		if strings.TrimSpace(turn.Text) != "" {
			parts = append(parts, map[string]any{"text": turn.Text})
		}
		if parts == nil {
			parts = make([]map[string]any, 0, len(turn.ToolCalls))
		}
		for _, tc := range turn.ToolCalls {
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"id":   "call_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
					"name": tc.Name,
					"args": tc.Input,
				},
			})
		}
		return parts
	}
	parts := thinkingPart()
	if turn.Text != "" {
		parts = append(parts, map[string]any{"text": turn.Text})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": ""})
	}
	return parts
}
