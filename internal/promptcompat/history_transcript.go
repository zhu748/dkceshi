package promptcompat

import (
        "fmt"
        "strings"
)

// CurrentInputContextFilename 仅作为历史兼容常量保留，不再用于真实上传。
// 真实上传的文件名由 history.Service 在每次上传时随机生成（参考真实 App 抓包
// 中 111.txt / 222.txt 这类用户风格命名）。
const CurrentInputContextFilename = "chat_context.txt"

// 早期实现使用了 "# Session Snapshot" / "Aggregated dialogue state and prior function-call results."
// 这种固定标题作为文件内容首行，是极强的内容指纹。现在改为更自然的引导句，
// 每次请求时还会拼接一个时间戳变化的尾巴，避免文件首部哈希稳定。
const historyTranscriptIntro = "The dialogue up to this point. Pick up from the most recent user message."

func BuildOpenAIHistoryTranscript(messages []any) string {
        return buildOpenAIHistoryTranscript(messages)
}

func BuildOpenAICurrentUserInputTranscript(text string) string {
        if strings.TrimSpace(text) == "" {
                return ""
        }
        return buildOpenAIHistoryTranscript([]any{
                map[string]any{"role": "user", "content": text},
        })
}

func BuildOpenAICurrentInputContextTranscript(messages []any) string {
        return buildOpenAIHistoryTranscript(messages)
}

func buildOpenAIHistoryTranscript(messages []any) string {
        if len(messages) == 0 {
                return ""
        }
        var b strings.Builder
        b.WriteString(historyTranscriptIntro)
        b.WriteString("\n\n")

        entry := 0
        for _, raw := range messages {
                msg, ok := raw.(map[string]any)
                if !ok {
                        continue
                }
                role := normalizeOpenAIRoleForPrompt(strings.ToLower(strings.TrimSpace(asString(msg["role"]))))
                content := strings.TrimSpace(buildOpenAIHistoryEntry(role, msg))
                if content == "" {
                        continue
                }
                entry++
                // 早期实现使用 "=== N. SYSTEM ===" 这种结构化分隔符，与真实 App 用户上传的
                // 文本文件格式差异极大。现在改成自然段标签 [system] / [user] / [assistant] / [tool]，
                // 既保持可读性，又避免被首行/分隔符特征命中。
                fmt.Fprintf(&b, "[%s]\n%s\n\n", roleLabelForHistory(role), content)
        }

        transcript := strings.TrimSpace(b.String())
        if transcript == "" {
                return ""
        }
        return transcript + "\n"
}

func buildOpenAIHistoryEntry(role string, msg map[string]any) string {
        switch role {
        case "assistant":
                return strings.TrimSpace(buildAssistantContentForPrompt(msg))
        case "tool", "function":
                return strings.TrimSpace(buildToolHistoryContent(msg))
        case "system", "user":
                return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
        default:
                return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
        }
}

func buildToolHistoryContent(msg map[string]any) string {
        content := strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
        parts := make([]string, 0, 2)
        if name := strings.TrimSpace(asString(msg["name"])); name != "" {
                parts = append(parts, "function="+name)
        }
        if callID := strings.TrimSpace(asString(msg["tool_call_id"])); callID != "" {
                parts = append(parts, "invocation_id="+callID)
        }
        header := ""
        if len(parts) > 0 {
                header = "[" + strings.Join(parts, " ") + "]"
        }
        switch {
        case header != "" && content != "":
                return header + "\n" + content
        case header != "":
                return header
        default:
                return content
        }
}

func roleLabelForHistory(role string) string {
        role = strings.ToLower(strings.TrimSpace(role))
        switch role {
        case "function":
                return "tool"
        case "":
                return "unknown"
        default:
                return role
        }
}
