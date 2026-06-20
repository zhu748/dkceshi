package toolcall

import (
        "encoding/json"
        "regexp"
        "strconv"
        "strings"
)

// claude_code_fallback.go
//
// Claude Code 客户端训练时偏好 "Tool: X\n<tool_input>{...}</tool_input>" 这种
// 工具调用格式，即使我们在 prompt 里明确要求用 <|DSML|tool_calls>...</|DSML|tool_calls>，
// 模型有时仍会输出这种格式。原始解析器只识别 DSML/XML 标签，遇到这种格式会
// 直接当成普通文本，导致工具调用被当成输出、对话卡住。
//
// 这里加一个预处理步骤：在标准 DSML 解析失败后，尝试把 Claude Code 格式
// 转换成 DSML 格式再解析一次。这样既保留严格的 DSML 解析逻辑，又能兜底
// 处理模型的偏好输出。

// claudeCodeToolPattern 匹配 Claude Code 风格的工具调用：
//
//      Tool: <工具名>
//      <tool_input>
//      {...JSON 参数...}
//      </tool_input>
//
// 或者紧凑形式：
//
//      Tool: <工具名>
//      <tool_input>{...JSON 参数...}</tool_input>
var claudeCodeToolPattern = regexp.MustCompile(
        `(?is)Tool:\s*([A-Za-z_][A-Za-z0-9_\-.]*)\s*\n\s*<tool_input>\s*(.*?)\s*</tool_input>`,
)

// claudeCodeFunctionCallPattern 匹配另一种常见变体：
//
//      **Calling:** <工具名>
//      {...JSON 参数...}
//
// 或：
//
//      <function_call>
//      {"name": "<工具名>", "arguments": {...}}
//      </function_call>
var claudeCodeFunctionCallPattern = regexp.MustCompile(
        `(?is)\*\*Calling:\*\*\s*([A-Za-z_][A-Za-z0-9_\-.]*)\s*\n\s*(\{.*?\})`,
)

// tryParseClaudeCodeFallback 尝试把 Claude Code 风格的工具调用转成 DSML 再解析。
// 返回 (calls, sawSyntax)：
//   - sawSyntax=true 表示文本里检测到 Claude Code 风格的工具调用语法
//   - calls 是转换后解析出的工具调用列表（可能为空，比如参数 JSON 解析失败）
func tryParseClaudeCodeFallback(text string) (ToolCallParseResult, bool) {
        result := ToolCallParseResult{}
        if !looksLikeClaudeCodeToolSyntax(text) {
                return result, false
        }

        dsml := convertClaudeCodeToDSML(text)
        if dsml == text {
                // 没有转换发生，说明正则没匹配
                result.SawToolCallSyntax = false
                return result, false
        }

        // 转换后的 DSML 走完整解析路径：先 normalizeDSMLToolCallMarkup 把
        // <|DSML|tool_calls> 转成 <tool_calls>，再 parseXMLToolCalls 解析。
        normalized, ok := normalizeDSMLToolCallMarkup(dsml)
        if !ok {
                result.SawToolCallSyntax = true
                return result, true
        }
        parsed := parseXMLToolCalls(normalized)
        if len(parsed) == 0 {
                // 检测到语法但解析失败（比如参数 JSON 不合法）
                result.SawToolCallSyntax = true
                return result, true
        }

        calls, rejectedNames := filterToolCallsDetailed(parsed)
        result.Calls = calls
        result.RejectedToolNames = rejectedNames
        result.RejectedByPolicy = len(rejectedNames) > 0 && len(calls) == 0
        result.SawToolCallSyntax = true
        return result, true
}

// looksLikeClaudeCodeToolSyntax 粗略检测文本是否包含 Claude Code 风格的工具调用语法。
func looksLikeClaudeCodeToolSyntax(text string) bool {
        return strings.Contains(text, "<tool_input>") ||
                strings.Contains(text, "**Calling:**") ||
                strings.Contains(text, "<function_call>")
}

// convertClaudeCodeToDSML 把 Claude Code 风格的工具调用转换成 DSML 格式。
// 如果文本里没有匹配项，原样返回。
func convertClaudeCodeToDSML(text string) string {
        // 转换 "Tool: X\n<tool_input>{...}</tool_input>" 形式
        text = claudeCodeToolPattern.ReplaceAllStringFunc(text, func(match string) string {
                sub := claudeCodeToolPattern.FindStringSubmatch(match)
                if len(sub) < 3 {
                        return match
                }
                name := strings.TrimSpace(sub[1])
                argsRaw := strings.TrimSpace(sub[2])
                return renderClaudeCodeAsDSML(name, argsRaw)
        })

        // 转换 "**Calling:** X\n{...}" 形式
        text = claudeCodeFunctionCallPattern.ReplaceAllStringFunc(text, func(match string) string {
                sub := claudeCodeFunctionCallPattern.FindStringSubmatch(match)
                if len(sub) < 3 {
                        return match
                }
                name := strings.TrimSpace(sub[1])
                argsRaw := strings.TrimSpace(sub[2])
                return renderClaudeCodeAsDSML(name, argsRaw)
        })

        return text
}

// renderClaudeCodeAsDSML 把单个工具调用渲染成 DSML 块。
// argsRaw 是原始 JSON 字符串（可能是 {"pattern":"*"} 这种）。
func renderClaudeCodeAsDSML(name, argsRaw string) string {
        // 尝试解析 JSON 参数，转成 DSML parameter 列表
        params, ok := parseArgsToDSMLParams(argsRaw)
        if !ok {
                // JSON 解析失败，把整个 argsRaw 当成单个 content 参数
                params = `<|DSML|parameter name="input"><![CDATA[` + escapeCDATA(argsRaw) + `]]></|DSML|parameter>`
        }
        return "<|DSML|tool_calls>\n  <|DSML|invoke name=\"" + escapeXMLAttribute(name) + "\">\n" +
                indentDSMLParams(params) + "\n" +
                "  </|DSML|invoke>\n</|DSML|tool_calls>"
}

// parseArgsToDSMLParams 把 JSON 参数对象转成 DSML parameter 列表字符串。
// 例如 {"pattern":"*"} -> <|DSML|parameter name="pattern"><![CDATA[*]]></|DSML|parameter>
func parseArgsToDSMLParams(argsRaw string) (string, bool) {
        argsRaw = strings.TrimSpace(argsRaw)
        if argsRaw == "" {
                return "", true
        }
        var args map[string]any
        if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
                return "", false
        }
        if len(args) == 0 {
                return "", true
        }
        // 收集 key 并排序，保证输出稳定
        keys := make([]string, 0, len(args))
        for k := range args {
                keys = append(keys, k)
        }
        // 简单排序（不用 sort 包避免额外 import，手动冒泡足够）
        for i := 0; i < len(keys); i++ {
                for j := i + 1; j < len(keys); j++ {
                        if keys[i] > keys[j] {
                                keys[i], keys[j] = keys[j], keys[i]
                        }
                }
        }
        var b strings.Builder
        for i, k := range keys {
                if i > 0 {
                        b.WriteString("\n")
                }
                b.WriteString(`<|DSML|parameter name="`)
                b.WriteString(escapeXMLAttribute(k))
                b.WriteString(`">`)
                b.WriteString(renderArgValue(args[k]))
                b.WriteString(`</|DSML|parameter>`)
        }
        return b.String(), true
}

// renderArgValue 把 JSON 值渲染成 DSML 节点内容。
func renderArgValue(v any) string {
        switch x := v.(type) {
        case string:
                return "<![CDATA[" + escapeCDATA(x) + "]]>"
        case nil:
                return ""
        case bool:
                if x {
                        return "true"
                }
                return "false"
        case float64:
                // JSON 数字默认解析成 float64，整数场景去掉小数点
                if x == float64(int64(x)) {
                        return strconv.FormatInt(int64(x), 10)
                }
                return strconv.FormatFloat(x, 'f', -1, 64)
        default:
                // object / array 用 JSON 字符串 + CDATA
                b, _ := json.Marshal(v)
                return "<![CDATA[" + escapeCDATA(string(b)) + "]]>"
        }
}

func escapeCDATA(s string) string {
        // CDATA 内部只需要转义 ]]>
        return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

func escapeXMLAttribute(s string) string {
        r := strings.NewReplacer(
                `&`, `&amp;`,
                `"`, `&quot;`,
                `<`, `&lt;`,
                `>`, `&gt;`,
        )
        return r.Replace(s)
}

func indentDSMLParams(params string) string {
        if params == "" {
                return "    " + `<|DSML|parameter name="content"></|DSML|parameter>`
        }
        lines := strings.Split(params, "\n")
        for i, line := range lines {
                if strings.TrimSpace(line) == "" {
                        continue
                }
                lines[i] = "    " + line
        }
        return strings.Join(lines, "\n")
}
