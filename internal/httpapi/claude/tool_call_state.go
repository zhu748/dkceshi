package claude

import (
	"ds2api/internal/promptcompat"
	"strings"
)

type claudeToolCallState = promptcompat.ToolCallLinker

func safeStringValue(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
