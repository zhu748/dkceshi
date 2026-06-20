package toolcall

import "strings"

// BuildToolCallInstructions generates the function-calling instruction block used
// by all adapters (OpenAI, Claude, Gemini).
//
// 风控与严格性平衡要点：
//   - 早期实现 17 条 CONTRACT + 5 例约 1500+ 字符且每次请求重复，是强内容指纹。
//   - 现在压缩到 7 条规则 + 1 个正确示例 + 5 个反例，保留原始规范的关键语义：
//     a) 工具调用块本身不能被 markdown fence / XML wrapper / 代码块包裹（关键，防解析器漏抓）
//     b) 工具调用块放在回复末尾，块后不能有文字（防止 prose after XML）
//     c) 块前可以有解释文字（模型先解释再调用是正常对话流）
//     d) 块本身的开头必须是 <|DSML|tool_calls>，不能漏开标签
//     e) 不允许 JSON / Markdown / prose 形式的工具调用
//     f) 不允许空参数值
//   - 5 个反例（Incorrect examples）从原始实现恢复，但措辞简化、不重新引入
//     "CRITICAL PARADIGM SHIFT" / "MANDATORY SELF-CHECK" 等大写关键词，
//     也不字面引用 tool_schema.txt，避免文件名交叉指纹。
//   - 反例中使用的工具名动态从当前请求的 toolNames 选，避免硬编码 Bash/Read
//     与实际请求工具不一致导致模型混淆。
//   - 保留 <|DSML|tool_calls> 标签语法不变，解析器依赖。
func BuildToolCallInstructions(toolNames []string) string {
	names := uniqueToolNames(toolNames)
	exampleToolName := pickExampleToolName(names)

	return `When you decide to call a function, end your response with this XML block:

<|DSML|tool_calls>
  <|DSML|invoke name="FUNCTION_NAME">
    <|DSML|parameter name="ARG_NAME"><![CDATA[VALUE]]></|DSML|parameter>
  </|DSML|invoke>
</|DSML|tool_calls>

Rules:
1) You may write explanatory text before the block, but the block must be the last thing in your response. Do not add any text, explanation, or greeting after </|DSML|tool_calls>.
2) The block itself must be bare XML. Do NOT wrap it in markdown fences, HTML, JSON, or any other code-block wrapper. The first non-whitespace characters of the block must be exactly <|DSML|tool_calls>.
3) Strings go inside <![CDATA[...]]>; numbers, booleans, and null stay as plain text. Every string parameter must be wrapped, even short ones.
4) Objects use nested XML elements inside the parameter body; arrays repeat <item> children.
5) Only use parameter names declared in the function schema. Do not invent fields, and never emit empty or whitespace-only parameter values. If a required value is unknown, ask the user instead.
6) Never output tool calls as JSON, Markdown, or prose. The only accepted form is the bare DSML XML block above.
7) If you are not calling a function, answer the user normally without emitting any DSML tags.

Avoid these incorrect patterns:

Incorrect 1 — text after the block:
  <|DSML|tool_calls>...</|DSML|tool_calls> I hope this helps.
Incorrect 2 — wrapped in markdown fences:
  ` + "```" + `xml
  <|DSML|tool_calls>...</|DSML|tool_calls>
  ` + "```" + `
Incorrect 3 — missing opening tag:
  <|DSML|invoke name="FUNCTION_NAME">...</|DSML|invoke>
  </|DSML|tool_calls>
Incorrect 4 — empty parameter value:
  <|DSML|tool_calls>
    <|DSML|invoke name="` + exampleToolName + `">
      <|DSML|parameter name="input"></|DSML|parameter>
    </|DSML|invoke>
  </|DSML|tool_calls>
Incorrect 5 — JSON or Markdown invocation instead of DSML:
  **Calling:** ` + exampleToolName + `
  {"input": "..."}

` + buildCorrectToolExamples(toolNames)
}

type promptToolExample struct {
	name   string
	params string
}

// pickExampleToolName 从当前请求的工具名里选一个用于反例展示。
// 优先选择已知有预置示例的工具（Bash/Read 等），其次选第一个工具名，
// 都没有时退化为占位名 "FUNCTION_NAME"。
func pickExampleToolName(names []string) string {
	for _, name := range names {
		if _, ok := exampleBasicParams(name); ok {
			return name
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "FUNCTION_NAME"
}

func buildCorrectToolExamples(toolNames []string) string {
	names := uniqueToolNames(toolNames)
	examples := make([]string, 0, 2)

	if single, ok := firstBasicExample(names); ok {
		examples = append(examples, "Correct example:\n"+renderToolExampleBlock([]promptToolExample{single}))
	}
	if len(examples) == 0 {
		return ""
	}
	return strings.Join(examples, "\n\n") + "\n"
}

func uniqueToolNames(toolNames []string) []string {
	names := make([]string, 0, len(toolNames))
	seen := map[string]bool{}
	for _, name := range toolNames {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

func firstBasicExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	// 没有命中预置示例时，退化为一个最简单的占位调用，保证格式示例始终存在。
	if len(names) > 0 {
		return promptToolExample{
			name:   names[0],
			params: wrapParameter("input", promptCDATA("...")),
		}, true
	}
	return promptToolExample{}, false
}

func renderToolExampleBlock(calls []promptToolExample) string {
	var b strings.Builder
	b.WriteString("<|DSML|tool_calls>\n")
	for _, call := range calls {
		b.WriteString(`  <|DSML|invoke name="`)
		b.WriteString(call.name)
		b.WriteString(`">` + "\n")
		b.WriteString(indentPromptParameters(call.params, "    "))
		b.WriteString("\n  </|DSML|invoke>\n")
	}
	b.WriteString("</|DSML|tool_calls>")
	return b.String()
}

func indentPromptParameters(body, indent string) string {
	if strings.TrimSpace(body) == "" {
		return indent + `<|DSML|parameter name="content"></|DSML|parameter>`
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = line
			continue
		}
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func wrapParameter(name, inner string) string {
	return `<|DSML|parameter name="` + name + `">` + inner + `</|DSML|parameter>`
}

func exampleBasicParams(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "Read":
		return wrapParameter("file_path", promptCDATA("README.md")), true
	case "Glob":
		return wrapParameter("pattern", promptCDATA("**/*.go")) + "\n" + wrapParameter("path", promptCDATA(".")), true
	case "read_file":
		return wrapParameter("path", promptCDATA("src/main.go")), true
	case "list_files":
		return wrapParameter("path", promptCDATA(".")), true
	case "search_files":
		return wrapParameter("query", promptCDATA("function-call parser")), true
	case "Bash", "execute_command":
		return wrapParameter("command", promptCDATA("pwd")), true
	case "exec_command":
		return wrapParameter("cmd", promptCDATA("pwd")), true
	case "Write":
		return wrapParameter("file_path", promptCDATA("notes.txt")) + "\n" + wrapParameter("content", promptCDATA("Hello world")), true
	case "write_to_file":
		return wrapParameter("path", promptCDATA("notes.txt")) + "\n" + wrapParameter("content", promptCDATA("Hello world")), true
	case "Edit":
		return wrapParameter("file_path", promptCDATA("README.md")) + "\n" + wrapParameter("old_string", promptCDATA("foo")) + "\n" + wrapParameter("new_string", promptCDATA("bar")), true
	case "MultiEdit":
		return wrapParameter("file_path", promptCDATA("README.md")) + "\n" + `<|DSML|parameter name="edits"><item><old_string>` + promptCDATA("foo") + `</old_string><new_string>` + promptCDATA("bar") + `</new_string></item></|DSML|parameter>`, true
	}
	return "", false
}

func promptCDATA(text string) string {
	if text == "" {
		return ""
	}
	if strings.Contains(text, "]]>") {
		return "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
	}
	return "<![CDATA[" + text + "]]>"
}
