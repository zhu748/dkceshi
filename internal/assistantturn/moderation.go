package assistantturn

import "strings"

// ModerationBlockedThinkingPhrase is the refusal text the upstream emits in the
// reasoning/thinking channel when the input content is intercepted by review.
const ModerationBlockedThinkingPhrase = "你好，这个问题我暂时无法回答，让我们换个话题再聊聊吧"

// IsModerationBlockedThinking reports whether the thinking chain carries the
// upstream moderation refusal text.
func IsModerationBlockedThinking(thinking string) bool {
	return strings.Contains(thinking, ModerationBlockedThinkingPhrase)
}
