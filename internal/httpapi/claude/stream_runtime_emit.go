package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"ds2api/internal/util"
)

func (s *claudeStreamRuntime) send(event string, v any) {
	// textEmitted also tracks whitespace-only blocks for rendering. Recovery
	// must instead know whether a visible answer has reached the client.
	if event == "content_block_delta" && !s.visibleTextEmitted {
		payload, _ := v.(map[string]any)
		delta, _ := payload["delta"].(map[string]any)
		text, _ := delta["text"].(string)
		s.visibleTextEmitted = strings.TrimSpace(text) != ""
	}
	b, _ := json.Marshal(v)
	_, _ = s.w.Write([]byte("event: "))
	_, _ = s.w.Write([]byte(event))
	_, _ = s.w.Write([]byte("\n"))
	_, _ = s.w.Write([]byte("data: "))
	_, _ = s.w.Write(b)
	_, _ = s.w.Write([]byte("\n\n"))
	if s.canFlush {
		_ = s.rc.Flush()
	}
}

func (s *claudeStreamRuntime) sendError(message string) {
	s.sendErrorWithCode(500, message, "internal_error")
}

func (s *claudeStreamRuntime) sendErrorWithCode(status int, message, code string) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "upstream stream error"
	}
	if code == "" {
		code = "internal_error"
	}
	errType := "api_error"
	if status == 429 {
		errType = "rate_limit_error"
	}
	s.send("error", map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errType,
			"message": msg,
			"code":    code,
			"param":   nil,
		},
	})
}

func (s *claudeStreamRuntime) sendPing() {
	s.send("ping", map[string]any{"type": "ping"})
}

func (s *claudeStreamRuntime) sendMessageStart() {
	inputTokens := s.countInputTokens(s.promptTokenText)
	if inputTokens == 0 {
		inputTokens = util.CountPromptTokens(fmt.Sprintf("%v", s.messages), s.model)
	}
	s.send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
}

func (s *claudeStreamRuntime) emitDeferredText() {
	text := cleanVisibleOutput(s.deferredText.String(), s.stripReferenceMarkers)
	s.deferredText.Reset()
	if text == "" {
		return
	}
	s.closeThinkingBlock()
	idx := s.nextBlockIndex
	s.nextBlockIndex++
	s.send("content_block_start", map[string]any{"type": "content_block_start", "index": idx, "content_block": map[string]any{"type": "text", "text": ""}})
	s.send("content_block_delta", map[string]any{"type": "content_block_delta", "index": idx, "delta": map[string]any{"type": "text_delta", "text": text}})
	s.send("content_block_stop", map[string]any{"type": "content_block_stop", "index": idx})
	s.textEmitted = true
}

// countInputTokens memoizes countClaudeInputTokensFromText per prompt text.
// message_start and finalize (and each retry attempt's finalize) count the
// same text; the memo keeps one full BPE encode per distinct prompt.
func (s *claudeStreamRuntime) countInputTokens(prompt string) int {
	if s == nil || prompt == "" {
		return 0
	}
	if s.inputTokenCounts == nil {
		s.inputTokenCounts = make(map[string]int, 2)
	}
	if v, ok := s.inputTokenCounts[prompt]; ok {
		return v
	}
	v := countClaudeInputTokensFromText(prompt, s.model)
	s.inputTokenCounts[prompt] = v
	return v
}

// promptTokenCountFor is the assistantturn.BuildOptions.PromptTokenCountFn
// adapter around countInputTokens.
func (s *claudeStreamRuntime) promptTokenCountFor(prompt string) (int, bool) {
	if s == nil || prompt == "" {
		return 0, false
	}
	return s.countInputTokens(prompt), true
}
