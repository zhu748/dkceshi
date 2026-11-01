package claude

func (s *claudeStreamRuntime) emitThinking(text string) {
	text = cleanVisibleOutput(text, s.stripReferenceMarkers)
	trimmed := text
	if trimmed == "" {
		return
	}
	s.thinking.WriteString(trimmed)
	s.closeTextBlock()
	if !s.thinkingBlockOpen {
		s.thinkingBlockIndex = s.nextBlockIndex
		s.nextBlockIndex++
		s.send("content_block_start", map[string]any{
			"type": "content_block_start", "index": s.thinkingBlockIndex,
			"content_block": map[string]any{"type": "thinking", "thinking": ""},
		})
		s.thinkingBlockOpen = true
	}
	s.send("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": s.thinkingBlockIndex,
		"delta": map[string]any{"type": "thinking_delta", "thinking": trimmed},
	})
}
