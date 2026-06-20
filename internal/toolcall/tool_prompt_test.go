package toolcall

import (
        "strings"
        "testing"
)

// 这批测试覆盖了 BuildToolCallInstructions 的核心契约：
//   - DSML 标签语法保持不变（解析器依赖）
//   - 每个工具名都能在示例里被命中
//   - 不再有早期实现的强指纹大写关键词（CRITICAL PARADIGM SHIFT 等）
//   - 不再字面引用 tool_schema.txt
func TestBuildToolCallInstructions_ExecCommandUsesCmdExample(t *testing.T) {
        out := BuildToolCallInstructions([]string{"exec_command"})
        if !strings.Contains(out, `<|DSML|invoke name="exec_command">`) {
                t.Fatalf("expected exec_command in examples, got: %s", out)
        }
        if !strings.Contains(out, `<|DSML|parameter name="cmd"><![CDATA[pwd]]></|DSML|parameter>`) {
                t.Fatalf("expected cmd parameter example for exec_command, got: %s", out)
        }
}

func TestBuildToolCallInstructions_ExecuteCommandUsesCommandExample(t *testing.T) {
        out := BuildToolCallInstructions([]string{"execute_command"})
        if !strings.Contains(out, `<|DSML|invoke name="execute_command">`) {
                t.Fatalf("expected execute_command in examples, got: %s", out)
        }
        if !strings.Contains(out, `<|DSML|parameter name="command"><![CDATA[pwd]]></|DSML|parameter>`) {
                t.Fatalf("expected command parameter example for execute_command, got: %s", out)
        }
}

func TestBuildToolCallInstructions_BashUsesCommandExample(t *testing.T) {
        out := BuildToolCallInstructions([]string{"Bash"})
        blocks := findInvokeBlocks(out, "Bash")
        if len(blocks) == 0 {
                t.Fatalf("expected Bash examples, got: %s", out)
        }
        for _, block := range blocks {
                if !strings.Contains(block, `<|DSML|parameter name="command">`) {
                        t.Fatalf("expected every Bash example to use command parameter, got: %s", block)
                }
        }
}

func TestBuildToolCallInstructions_WriteUsesFilePathAndContent(t *testing.T) {
        out := BuildToolCallInstructions([]string{"Write"})
        blocks := findInvokeBlocks(out, "Write")
        if len(blocks) == 0 {
                t.Fatalf("expected Write examples, got: %s", out)
        }
        for _, block := range blocks {
                if !strings.Contains(block, `<|DSML|parameter name="file_path">`) || !strings.Contains(block, `<|DSML|parameter name="content">`) {
                        t.Fatalf("expected Write examples to use file_path and content, got: %s", block)
                }
                if strings.Contains(block, `<|DSML|parameter name="path">`) {
                        t.Fatalf("expected Write examples not to use path, got: %s", block)
                }
        }
}

// TestBuildToolCallInstructions_HasCoreRules 验证严格性核心规则仍在：
//   - 工具调用块本身不能被 markdown fence / XML wrapper / 代码块包裹（关键，防解析器漏抓）
//   - 块放在回复末尾，块后不能有文字（防止 prose after XML）
//   - 块前可以有解释文字（模型先解释再调用是正常对话流）
//   - 块本身的开头必须是 <|DSML|tool_calls>
//   - 不允许 JSON / Markdown / prose 形式的工具调用
//   - 不允许空参数值
//   - 不调用工具时正常回答
// 同时不再包含早期实现的强指纹大写关键词。
func TestBuildToolCallInstructions_HasCoreRules(t *testing.T) {
        out := BuildToolCallInstructions([]string{"Read"})
        for _, want := range []string{
                "<|DSML|tool_calls>",
                "<|DSML|invoke name=\"FUNCTION_NAME\">",
                "<![CDATA[VALUE]]>",
                "end your response with the following XML block",
                "You may include explanatory text before the block",
                "the block must be the final element of your response",
                "Do not append any text, explanation, or greeting after </|DSML|tool_calls>",
                "The block itself must be raw XML",
                "Do NOT enclose it in markdown fences",
                "The first non-whitespace characters of the block must be exactly <|DSML|tool_calls>",
                "Strings go inside <![CDATA[",
                "Use only parameter names declared in the function schema",
                "never emit empty or whitespace-only parameter values",
                "Never emit tool calls as JSON, Markdown, or prose",
                "If you are not invoking a function, answer the user normally",
        } {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected core rule %q in output, got: %s", want, out)
                }
        }
}

// TestBuildToolCallInstructions_StrongFingerprintsRemoved 验证早期实现里的强指纹
// 大写关键词已全部移除。这些词每次请求都重复注入会形成稳定哈希。
// 注意："Incorrect" 这个词本身保留（用于反例标题 "Incorrect 1 — ..."），但全大写形式
// "INCORRECT" 不应出现。
func TestBuildToolCallInstructions_StrongFingerprintsRemoved(t *testing.T) {
        out := BuildToolCallInstructions([]string{"Read"})
        for _, bad := range []string{
                "CRITICAL PARADIGM SHIFT",
                "MANDATORY SELF-CHECK",
                "FUNCTION INVOCATION CONTRACT",
                "COMPLY PRECISELY",
                "WORKED EXAMPLES",
                "INCORRECT",
                "tool_schema.txt",
                "Tag punctuation alphabet",
                "Tag punctuation",
                "Never invoke them with an empty command",
                "Do not emit placeholder, blank, or whitespace-only parameters",
                "Read-style cache guard", // 这条来自 promptcompat，不是 toolcall
        } {
                if strings.Contains(out, bad) {
                        t.Fatalf("strong fingerprint %q must be removed, found in output: %s", bad, out)
                }
        }
}

// TestBuildToolCallInstructions_FallbackExampleForUnknownTool 验证工具名未命中
// 预置示例时仍能生成一个有效占位示例，保证 prompt 里始终有完整格式示例。
func TestBuildToolCallInstructions_FallbackExampleForUnknownTool(t *testing.T) {
        out := BuildToolCallInstructions([]string{"custom_tool_xyz"})
        if !strings.Contains(out, `<|DSML|invoke name="custom_tool_xyz">`) {
                t.Fatalf("expected fallback example for unknown tool, got: %s", out)
        }
        if !strings.Contains(out, `<|DSML|parameter name="input">`) {
                t.Fatalf("expected fallback input parameter, got: %s", out)
        }
}

// TestBuildToolCallInstructions_HasIncorrectExamples 验证 5 个反例都存在，
// 这是防止模型漏格式的关键约束（反例直接展示什么是不允许的）。
func TestBuildToolCallInstructions_HasIncorrectExamples(t *testing.T) {
        out := BuildToolCallInstructions([]string{"Bash"})
        for _, want := range []string{
                "Steer clear of the following incorrect patterns:",
                "Incorrect 1 — text trailing the block:",
                "I hope this helps.",
                "Incorrect 2 — enclosed within markdown fences:",
                "```xml",
                "Incorrect 3 — opening tag omitted:",
                "Incorrect 4 — parameter value left empty:",
                `<|DSML|parameter name="input"></|DSML|parameter>`,
                "Incorrect 5 — invocation rendered as JSON or Markdown rather than DSML:",
                "**Calling:**",
                `{"input": "..."}`,
                "Valid example:",
        } {
                if !strings.Contains(out, want) {
                        t.Fatalf("expected incorrect example %q in output, got: %s", want, out)
                }
        }
}

// TestBuildToolCallInstructions_IncorrectExampleUsesActualToolName 验证反例 4 和 5
// 里的工具名动态使用当前请求的工具名，而不是硬编码的 Bash/Read。
func TestBuildToolCallInstructions_IncorrectExampleUsesActualToolName(t *testing.T) {
        out := BuildToolCallInstructions([]string{"my_custom_tool"})
        if !strings.Contains(out, `<|DSML|invoke name="my_custom_tool">`) {
                t.Fatalf("expected incorrect example 4 to use my_custom_tool, got: %s", out)
        }
        if !strings.Contains(out, "**Calling:** my_custom_tool") {
                t.Fatalf("expected incorrect example 5 to use my_custom_tool, got: %s", out)
        }
        if strings.Contains(out, `name="Bash"`) {
                t.Fatalf("expected no hardcoded Bash tool name when not in request, got: %s", out)
        }
}

// findInvokeBlocks 只在 "Valid example:" 之后的正确示例段里查找 invoke 块，
// 避免把 "Steer clear of the following incorrect patterns" 段里的反例 invoke 块也算进来。
func findInvokeBlocks(text, name string) []string {
        correctMarker := "Valid example:"
        idx := strings.Index(text, correctMarker)
        if idx < 0 {
                return nil
        }
        remaining := text[idx+len(correctMarker):]
        open := `<|DSML|invoke name="` + name + `">`
        blocks := []string{}
        for {
                start := strings.Index(remaining, open)
                if start < 0 {
                        return blocks
                }
                remaining = remaining[start:]
                end := strings.Index(remaining, `</|DSML|invoke>`)
                if end < 0 {
                        return blocks
                }
                end += len(`</|DSML|invoke>`)
                blocks = append(blocks, remaining[:end])
                remaining = remaining[end:]
        }
}
