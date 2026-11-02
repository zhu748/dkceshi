package promptcompat

import (
	"regexp"
	"strings"
)

// Recognize catalog structure rather than ordinary discussion or instructions
// about tools. Keep this shared by routing, prompt injection and output holding.
var toolCatalogHeading = regexp.MustCompile(`(?i)^(?:#{1,6}[ \t]+)?(?:available (?:tools|functions)|(?:tool|function) (?:catalog|definitions)|tools|functions|you have access to (?:these|the following) tools|可用工具|工具列表|工具定义|可用函数)[ \t]*(?:[:：][ \t]*(.*)|$)`)
var toolCatalogEntry = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]*(?:[ \t]*[,(:]|[ \t]*$)`)
var labeledToolEntry = regexp.MustCompile(`(?i)^tool:[ \t]*[a-zA-Z_][a-zA-Z0-9_.-]*[ \t]*$`)

func textMentionsTools(text string) bool {
	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if labeledToolEntry.MatchString(line) {
			for _, next := range lines[i+1 : min(i+5, len(lines))] {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(next)), "parameters:") {
					return true
				}
			}
		}
		match := toolCatalogHeading.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		entry := strings.TrimSpace(match[1])
		if entry == "" {
			for _, next := range lines[i+1:] {
				if entry = strings.TrimSpace(next); entry != "" {
					break
				}
			}
		}
		entry = strings.TrimSpace(strings.TrimLeft(entry, "-*"))
		switch strings.ToLower(entry) {
		case "", "none", "none.", "null", "[]", "{}", "n/a", "无", "没有":
			continue
		}
		if toolCatalogEntry.MatchString(entry) {
			return true
		}
	}
	return false
}
