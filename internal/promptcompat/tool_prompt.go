package promptcompat

import (
        "encoding/json"
        "fmt"
        "strings"
        "unicode"

        "ds2api/internal/toolcall"
)

// CurrentToolsContextFilename 仅作为历史兼容常量保留，不再用于真实上传。
// 真实上传的文件名由 history.Service 在每次上传时随机生成。
const CurrentToolsContextFilename = "tool_schema.txt"

// 早期实现使用 "# Callable Surface" / "Catalogue of invokable functions..." 这种
// 固定标题作为工具描述文件首行，是极强的内容指纹。现在改为更自然的引导句。
const toolsTranscriptIntro = "You can call the following functions in this turn."

type toolPromptParts struct {
        Descriptions string
        Instructions string
        Names        []string
}

func injectToolPrompt(messages []map[string]any, tools []any, policy ToolChoicePolicy) ([]map[string]any, []string) {
        return injectToolPromptWithDescriptions(messages, tools, policy, true)
}

func injectToolPromptInstructionsOnly(messages []map[string]any, tools []any, policy ToolChoicePolicy) ([]map[string]any, []string) {
        return injectToolPromptWithDescriptions(messages, tools, policy, false)
}

func injectToolPromptWithDescriptions(messages []map[string]any, tools []any, policy ToolChoicePolicy, includeDescriptions bool) ([]map[string]any, []string) {
        if policy.IsNone() {
                return messages, nil
        }
        parts := buildToolPromptParts(tools, policy)
        if parts.Instructions == "" {
                return messages, parts.Names
        }
        toolPrompt := parts.Instructions
        if includeDescriptions && parts.Descriptions != "" {
                // 模式 A：工具描述直接内联在 system 消息里，需要加引导句告诉模型这些是可用工具。
                toolPrompt = "You can call the following functions in this turn:\n\n" + parts.Descriptions + "\n\n" + toolPrompt
        } else if !includeDescriptions && parts.Descriptions != "" {
                // 模式 B 路径下，工具描述已单独上传为文件，这里只在 system 里追加一句
                // 引导语，避免再字面提到具体文件名（如 tool_schema.txt），降低文字指纹。
                toolPrompt = "The attached file lists the invokable function definitions and parameter contracts for this turn. Trust exclusively the functions and parameter shapes enumerated there and do not improvise any that are not declared.\n\n" + toolPrompt
        }

        for i := range messages {
                if messages[i]["role"] == "system" {
                        old, _ := messages[i]["content"].(string)
                        messages[i]["content"] = strings.TrimSpace(old + "\n\n" + toolPrompt)
                        return messages, parts.Names
                }
        }
        messages = append([]map[string]any{{"role": "system", "content": toolPrompt}}, messages...)
        return messages, parts.Names
}

func buildToolPromptParts(tools []any, policy ToolChoicePolicy) toolPromptParts {
        toolSchemas := make([]string, 0, len(tools))
        names := make([]string, 0, len(tools))
        isAllowed := func(name string) bool {
                if strings.TrimSpace(name) == "" {
                        return false
                }
                if len(policy.Allowed) == 0 {
                        return true
                }
                _, ok := policy.Allowed[name]
                return ok
        }

        for _, t := range tools {
                tool, ok := t.(map[string]any)
                if !ok {
                        continue
                }
                name, desc, schema := toolcall.ExtractToolMeta(tool)
                name = strings.TrimSpace(name)
                if !isAllowed(name) {
                        continue
                }
                names = append(names, name)
                if desc == "" {
                        desc = "No description available"
                }
                b, _ := json.Marshal(schema)
                // 早期实现使用 "Callable: X\nSynopsis: Y\nContract: Z" 这种结构化前缀，
                // 是强内容指纹。现在改为更自然的 "name: X\ndescription: Y\nschema: Z"。
                toolSchemas = append(toolSchemas, fmt.Sprintf("name: %s\ndescription: %s\nschema: %s", name, desc, string(b)))
        }
        if len(toolSchemas) == 0 {
                return toolPromptParts{Names: names}
        }
        // descriptions 只包含工具 schema 列表本身，不带引导句。
        // 引导句由调用方根据上下文决定：
        // - 模式 A（injectToolPrompt）：在 system 消息里直接拼接，需要引导句
        // - 模式 B（BuildOpenAIToolsContextTranscript）：文件级已有 toolsTranscriptIntro，不需要重复
        descriptions := strings.Join(toolSchemas, "\n\n")
        instructions := toolcall.BuildToolCallInstructions(names)
        if hasReadLikeTool(names) {
                instructions += "\n\nRead-style cache guard: when a Read/read_file-style tool result reports the file is unchanged, already present in prior context, or otherwise provides no fresh file body, treat that result as missing content. Do not keep re-issuing the same read for the missing body. Ask for a full-content read if the tool supports it, or tell the user the file contents need to be supplied again."
        }
        if policy.Mode == ToolChoiceRequired {
                instructions += "\n7) For this response, you MUST make at least one call to a tool from the allowed list."
        }
        if policy.Mode == ToolChoiceForced && strings.TrimSpace(policy.ForcedName) != "" {
                instructions += "\n7) For this response, you MUST make exactly this call to a tool named: " + strings.TrimSpace(policy.ForcedName)
                instructions += "\n8) Do not make any other tool call."
        }
        return toolPromptParts{
                Descriptions: descriptions,
                Instructions: instructions,
                Names:        names,
        }
}

func BuildOpenAIToolsContextTranscript(toolsRaw any, policy ToolChoicePolicy) (string, []string) {
        if policy.IsNone() {
                return "", nil
        }
        tools, ok := toolsRaw.([]any)
        if !ok || len(tools) == 0 {
                return "", nil
        }
        parts := buildToolPromptParts(tools, policy)
        if strings.TrimSpace(parts.Descriptions) == "" {
                return "", parts.Names
        }
        var b strings.Builder
        b.WriteString(toolsTranscriptIntro)
        b.WriteString("\n\n")
        b.WriteString(parts.Descriptions)
        b.WriteString("\n")
        return b.String(), parts.Names
}

func hasReadLikeTool(names []string) bool {
        for _, name := range names {
                switch normalizeToolNameForGuard(name) {
                case "read", "readfile":
                        return true
                }
        }
        return false
}

func normalizeToolNameForGuard(name string) string {
        var b strings.Builder
        for _, r := range strings.ToLower(strings.TrimSpace(name)) {
                if unicode.IsLetter(r) || unicode.IsDigit(r) {
                        b.WriteRune(r)
                }
        }
        return b.String()
}
