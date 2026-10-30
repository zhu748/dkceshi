package promptcompat

import (
	"strings"

	"ds2api/internal/config"
)

const (
	ThinkingInjectionMarker        = "Reasoning Effort: Beyond maximum — exhaustive, relentless, and uncompromising."
	DefaultThinkingInjectionPrompt = ThinkingInjectionMarker + "\n" +
		"You MUST reason with the utmost depth and rigor, leaving absolutely nothing to chance: exhaustively decompose the problem into its most fundamental components, trace every causal chain to its root, and resolve the underlying cause rather than any surface symptom. Do not stop reasoning until you have independently verified the solution from multiple angles and are certain that no assumption remains unchecked and no error remains undiscovered."
)

// ThinkingInjectionReader 由配置存储实现，提供「思考注入」设置。与
// ToolReminderReader 一样，它以窄接口的形式在请求路径上按需断言：真实实现只有
// config.Store，各协议包里的轻量 ConfigReader 桩无需实现它（未实现即退化为
// 「不注入」，与历史行为一致）。
type ThinkingInjectionReader interface {
	ThinkingInjectionEnabled() bool
	ThinkingInjectionPrompt() string
}

// 真实配置存储必须实现该能力，否则「思考注入」会在所有出口上静默失效。
var _ ThinkingInjectionReader = (*config.Store)(nil)

// ApplyThinkingInjection applies the 「思考注入」 setting to a normalized request:
// when enabled for a thinking request, its generated system instruction is the
// first text sent upstream. Client messages and the configured text are preserved.
//
// Every protocol surface calls it once, before the current-input-file step, so
// the setting behaves the same on OpenAI Chat / Responses, Gemini and Claude
// instead of being wired into some surfaces only.
func ApplyThinkingInjection(store any, stdReq StandardRequest) StandardRequest {
	reader, ok := store.(ThinkingInjectionReader)
	if !ok || !reader.ThinkingInjectionEnabled() || !stdReq.Thinking {
		return stdReq
	}
	injectionPrompt := strings.TrimSpace(reader.ThinkingInjectionPrompt())
	if injectionPrompt == "" || injectionPrompt == legacyThinkingInjectionPrompt {
		injectionPrompt = DefaultThinkingInjectionPrompt
	}
	if stdReq.ThinkingInjectionPrompt == injectionPrompt && strings.HasPrefix(stdReq.FinalPrompt, thinkingSystemPrefix(injectionPrompt)) {
		return stdReq
	}
	stdReq.ThinkingInjectionPrompt = injectionPrompt
	// The rebuild replays stdReq.Messages through the same surface-aware builder
	// the adapter used (StandardRequest.BuildPromptFromMessages), so an exit that
	// opted out of the system-text tool branch does not get it back here — and,
	// because Messages is the list FinalPrompt was rendered from, no turn can go
	// missing on the way through.
	finalPrompt, toolNames := stdReq.BuildPromptFromMessages(stdReq.Messages)
	if len(toolNames) == 0 && len(stdReq.ToolNames) > 0 {
		toolNames = stdReq.ToolNames
	}
	// The shared builder marks generated tool syntax before merging user content.
	stdReq.FinalPrompt = finalPrompt
	// PromptTokenText is the text the usage reports count, and every exit sets it
	// to the prompt it renders. Leaving it at the pre-injection value would make
	// the reported prompt_tokens / input_tokens describe a prompt that was never
	// sent — the injection is exactly the difference.
	stdReq.PromptTokenText = stdReq.FinalPrompt
	stdReq.ToolNames = toolNames
	return stdReq
}

// withPromptInstructions composes generated instructions in one order on normal
// requests and file continuations: thinking first, search next, then the client
// conversation. Strip only our exact leading prefix, never text inside a message.
func (r StandardRequest) withPromptInstructions(text string) string {
	prefix := ""
	if r.ThinkingInjectionPrompt != "" {
		prefix = thinkingSystemPrefix(r.ThinkingInjectionPrompt)
		text = strings.TrimPrefix(text, prefix)
	}
	if r.Search {
		text = prependBuiltinSearchPrompt(text, r.ToolMarker)
	}
	if text == "" {
		return ""
	}
	return prefix + text
}

func thinkingSystemPrefix(injectionPrompt string) string {
	return "<｜System｜>" + injectionPrompt + "\n\n"
}

const legacyThinkingInjectionPrompt = `text: You are a helpful software engineer assistant.
description: Bootstrap with shell/read, then expose the full Standard tool catalog after the first durable tool call.
**when you thought, start with "we need..."**`
