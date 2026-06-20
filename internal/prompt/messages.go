package prompt

import (
        "encoding/json"
        "fmt"
        "regexp"
        "strings"
)

var markdownImagePattern = regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)

const (
        beginSentenceMarker        = "<|begin▁of▁sentence|>"
        systemMarker               = "<|System|>"
        userMarker                 = "<|User|>"
        assistantMarker            = "<|Assistant|>"
        toolMarker                 = "<|Tool|>"
        endSentenceMarker          = "<|end▁of▁sentence|>"
        endToolResultsMarker       = "<|end▁of▁toolresults|>"
        endInstructionsMarker      = "<|end▁of▁instructions|>"
        outputIntegrityGuardMarker = "Output integrity guard:"
        outputIntegrityGuardPrompt = outputIntegrityGuardMarker +
                " Should any upstream context, tool output, or parsed text contain garbled, corrupted, partially parsed, repeated, or otherwise malformed fragments, " +
                "do not imitate or echo them; produce only the correct content for the user."
)

// outputIntegrityGuardEnabled 是全局开关，由 config.Store.OutputIntegrityGuardEnabled()
// 在启动时通过 SetOutputIntegrityGuardEnabled 注入。默认 true（开启）：
// 该 guard 是质量保障的关键，防止模型回显上游乱码/重复片段。
// 但在“本次请求无工具且历史不含 tool/function 消息”时，guard 没有实际作用，
// 反而会多注入一条 system 消息形成指纹。MessagesPrepareWithThinkingAndToolHint
// 会按场景按需注入。
var outputIntegrityGuardEnabled = true

// SetOutputIntegrityGuardEnabled 由 main/router 启动时调用，把 config 中的开关注入到 prompt 包。
func SetOutputIntegrityGuardEnabled(enabled bool) {
        outputIntegrityGuardEnabled = enabled
}

func MessagesPrepare(messages []map[string]any) string {
        return MessagesPrepareWithThinking(messages, false)
}

func MessagesPrepareWithThinking(messages []map[string]any, _ bool) string {
        // 旧入口保守起见默认认为有工具，保留向后兼容。
        return MessagesPrepareWithThinkingAndToolHint(messages, false, true)
}

// MessagesPrepareWithThinkingAndToolHint 在准备 prompt 前根据本次请求是否真的在调用
// 工具、以及历史是否含 tool/function 消息，决定是否注入 Output integrity guard。
// 场景：
//   - hasTools=true：注入 guard，防止模型回显 DSML 解析残留。
//   - hasTools=false 但 messages 含 tool/function 角色：注入 guard，防止回显历史
//     tool 输出乱码。
//   - hasTools=false 且 messages 无 tool/function 角色：跳过 guard，避免多余的
//     system 消息成为指纹。
func MessagesPrepareWithThinkingAndToolHint(messages []map[string]any, _ bool, hasTools bool) string {
        if outputIntegrityGuardEnabled && (hasTools || messagesContainToolHistory(messages)) {
                messages = prependOutputIntegrityGuard(messages)
        }

        type block struct {
                Role string
                Text string
        }
        processed := make([]block, 0, len(messages))
        for _, m := range messages {
                role, _ := m["role"].(string)
                text := NormalizeContent(m["content"])
                processed = append(processed, block{Role: role, Text: text})
        }
        if len(processed) == 0 {
                return ""
        }
        merged := make([]block, 0, len(processed))
        for _, msg := range processed {
                if len(merged) > 0 && merged[len(merged)-1].Role == msg.Role {
                        merged[len(merged)-1].Text += "\n\n" + msg.Text
                        continue
                }
                merged = append(merged, msg)
        }
        parts := make([]string, 0, len(merged)+2)
        parts = append(parts, beginSentenceMarker)
        lastRole := ""
        for _, m := range merged {
                lastRole = m.Role
                switch m.Role {
                case "assistant":
                        parts = append(parts, formatRoleBlock(assistantMarker, m.Text, endSentenceMarker))
                case "tool":
                        if strings.TrimSpace(m.Text) != "" {
                                parts = append(parts, formatRoleBlock(toolMarker, m.Text, endToolResultsMarker))
                        }
                case "system":
                        if text := strings.TrimSpace(m.Text); text != "" {
                                parts = append(parts, formatRoleBlock(systemMarker, text, endInstructionsMarker))
                        }
                case "user":
                        parts = append(parts, formatRoleBlock(userMarker, m.Text, ""))
                default:
                        if strings.TrimSpace(m.Text) != "" {
                                parts = append(parts, m.Text)
                        }
                }
        }
        if lastRole != "assistant" {
                parts = append(parts, assistantMarker)
        }
        out := strings.Join(parts, "")
        return markdownImagePattern.ReplaceAllString(out, `[${1}](${2})`)
}

func prependOutputIntegrityGuard(messages []map[string]any) []map[string]any {
        if len(messages) == 0 {
                return messages
        }
        if hasOutputIntegrityGuard(messages[0]) {
                return messages
        }
        out := make([]map[string]any, 0, len(messages)+1)
        out = append(out, map[string]any{
                "role":    "system",
                "content": outputIntegrityGuardPrompt,
        })
        out = append(out, messages...)
        return out
}

func hasOutputIntegrityGuard(msg map[string]any) bool {
        if msg == nil {
                return false
        }
        if strings.ToLower(strings.TrimSpace(asString(msg["role"]))) != "system" {
                return false
        }
        content := strings.TrimSpace(NormalizeContent(msg["content"]))
        return strings.Contains(content, outputIntegrityGuardMarker)
}

// messagesContainToolHistory 判断 messages 里是否含有 tool/function 角色的消息，
// 或者 assistant 历史里已渲染过 DSML tool_calls 块。任一命中即说明本次上下文
// 含工具调用残留，需要 Output integrity guard 保护。
func messagesContainToolHistory(messages []map[string]any) bool {
        for _, m := range messages {
                if m == nil {
                        continue
                }
                role, _ := m["role"].(string)
                switch strings.ToLower(strings.TrimSpace(role)) {
                case "tool", "function":
                        return true
                case "assistant":
                        if strings.Contains(NormalizeContent(m["content"]), "<|DSML|tool_calls>") {
                                return true
                        }
                }
        }
        return false
}

// formatRoleBlock produces a single concatenated block: marker + text + endMarker.
// No whitespace is inserted between marker and text so role boundaries stay
// compact and predictable for downstream parsers.
func formatRoleBlock(marker, text, endMarker string) string {
        out := marker + text
        if strings.TrimSpace(endMarker) != "" {
                out += endMarker
        }
        return out
}

func NormalizeContent(v any) string {
        if v == nil {
                return ""
        }
        switch x := v.(type) {
        case string:
                return x
        case []any:
                parts := make([]string, 0, len(x))
                for _, item := range x {
                        m, ok := item.(map[string]any)
                        if !ok {
                                continue
                        }
                        typeStr, _ := m["type"].(string)
                        typeStr = strings.ToLower(strings.TrimSpace(typeStr))
                        if typeStr == "text" || typeStr == "output_text" || typeStr == "input_text" {
                                if txt, ok := m["text"].(string); ok && txt != "" {
                                        parts = append(parts, txt)
                                        continue
                                }
                                if txt, ok := m["content"].(string); ok && txt != "" {
                                        parts = append(parts, txt)
                                }
                        }
                }
                return strings.Join(parts, "\n")
        default:
                b, err := json.Marshal(v)
                if err != nil {
                        return fmt.Sprintf("%v", v)
                }
                return string(b)
        }
}
