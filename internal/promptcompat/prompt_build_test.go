package promptcompat

import (
        "strings"
        "testing"
)

func TestBuildOpenAIFinalPrompt_HandlerPathIncludesToolRoundtripSemantics(t *testing.T) {
        messages := []any{
                map[string]any{"role": "user", "content": "查北京天气"},
                map[string]any{
                        "role": "assistant",
                        "tool_calls": []any{
                                map[string]any{
                                        "id": "call_1",
                                        "function": map[string]any{
                                                "name":      "get_weather",
                                                "arguments": "{\"city\":\"beijing\"}",
                                        },
                                },
                        },
                },
                map[string]any{
                        "role":         "tool",
                        "tool_call_id": "call_1",
                        "name":         "get_weather",
                        "content":      map[string]any{"temp": 18, "condition": "sunny"},
                },
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "get_weather",
                                "description": "Get weather",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        finalPrompt, toolNames := buildOpenAIFinalPrompt(messages, tools, "", false)
        if len(toolNames) != 1 || toolNames[0] != "get_weather" {
                t.Fatalf("unexpected tool names: %#v", toolNames)
        }
        if !strings.Contains(finalPrompt, `"condition":"sunny"`) {
                t.Fatalf("handler finalPrompt should preserve tool output content: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "<|DSML|tool_calls>") {
                t.Fatalf("handler finalPrompt should preserve assistant tool history: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, `<|DSML|invoke name="get_weather">`) {
                t.Fatalf("handler finalPrompt should include tool name history: %q", finalPrompt)
        }
}

func TestBuildOpenAIFinalPrompt_VercelPreparePathKeepsFinalAnswerInstruction(t *testing.T) {
        messages := []any{
                map[string]any{"role": "system", "content": "You are helpful"},
                map[string]any{"role": "user", "content": "请调用工具"},
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "search",
                                "description": "search docs",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
        // 瘦身后的 prompt 不再有 "Reminder: The ONLY sanctioned way..." 这种长锚点句，
        // 改为验证核心 DSML 标签和简化的 Rules 段。
        if !strings.Contains(finalPrompt, "<|DSML|tool_calls>") {
                t.Fatalf("vercel prepare finalPrompt missing DSML tool_calls tag: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "Never output tool calls as JSON, Markdown, or prose") {
                t.Fatalf("vercel prepare finalPrompt missing no-fence xml instruction: %q", finalPrompt)
        }
        if strings.Contains(finalPrompt, "```json") {
                t.Fatalf("vercel prepare finalPrompt should not require fenced tool calls: %q", finalPrompt)
        }
}

func TestBuildOpenAIPromptWithToolInstructionsOnlyOmitsSchemas(t *testing.T) {
        messages := []any{
                map[string]any{"role": "system", "content": "You are helpful"},
                map[string]any{"role": "user", "content": "请调用工具"},
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "search",
                                "description": "search docs",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        finalPrompt, toolNames := BuildOpenAIPromptWithToolInstructionsOnly(messages, tools, "", DefaultToolChoicePolicy(), false)
        if len(toolNames) != 1 || toolNames[0] != "search" {
                t.Fatalf("unexpected tool names: %#v", toolNames)
        }
        if strings.Contains(finalPrompt, "You can call the following functions in this turn:") || strings.Contains(finalPrompt, "description: search docs") || strings.Contains(finalPrompt, "schema:") {
                t.Fatalf("function descriptions should be externalized, got: %q", finalPrompt)
        }
        // 瘦身后不再字面提到 tool_schema.txt，改为更自然的引导句。
        if !strings.Contains(finalPrompt, "The attached file lists the invokable function definitions") {
                t.Fatalf("expected instructions-only prompt to point model at tools file, got: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "<|DSML|tool_calls>") || !strings.Contains(finalPrompt, "Never output tool calls as JSON, Markdown, or prose") {
                t.Fatalf("expected function-call format instructions to remain in live prompt, got: %q", finalPrompt)
        }
}

func TestBuildOpenAIToolsContextTranscriptContainsOnlyDescriptions(t *testing.T) {
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "search",
                                "description": "search docs",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        transcript, toolNames := BuildOpenAIToolsContextTranscript(tools, DefaultToolChoicePolicy())
        if len(toolNames) != 1 || toolNames[0] != "search" {
                t.Fatalf("unexpected tool names: %#v", toolNames)
        }
        // 风控优化后，工具描述文件不再使用 "You can call the following functions in this turn." 这种结构化标题，
// 改为更自然的引导句 "You can call the following functions in this turn."。
        for _, want := range []string{"You can call the following functions in this turn.", "name: search", "description: search docs", `schema: {"type":"object"}`} {
                if !strings.Contains(transcript, want) {
                        t.Fatalf("expected tools transcript to contain %q, got: %q", want, transcript)
                }
        }
        if strings.Contains(transcript, "<|DSML|tool_calls>") {
                t.Fatalf("tools transcript should not duplicate format instructions, got: %q", transcript)
        }
}

func TestBuildOpenAIFinalPromptPrependsOutputIntegrityGuard(t *testing.T) {
        messages := []any{
                map[string]any{"role": "system", "content": "You are helpful"},
                map[string]any{"role": "user", "content": "请调用工具"},
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "search",
                                "description": "search docs",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        // Output integrity guard 默认开启，final prompt 应包含 guard。
        finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
        if !strings.Contains(finalPrompt, "Output integrity guard") {
                t.Fatalf("output integrity guard should be enabled by default, got: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "<|DSML|tool_calls>") {
                t.Fatalf("expected tool instructions in final prompt, got: %q", finalPrompt)
        }
}

func TestBuildOpenAIFinalPromptReadLikeToolIncludesCacheGuard(t *testing.T) {
        messages := []any{
                map[string]any{"role": "user", "content": "请读取文件"},
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "read_file",
                                "description": "Read a file",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
        if !strings.Contains(finalPrompt, "Read-style cache guard") {
                t.Fatalf("read-like tool prompt missing cache guard: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "treat that result as missing content") {
                t.Fatalf("read-like tool prompt missing no-body handling: %q", finalPrompt)
        }
        if !strings.Contains(finalPrompt, "Do not keep re-issuing the same read for the missing body") {
                t.Fatalf("read-like tool prompt missing loop guard: %q", finalPrompt)
        }
}

func TestBuildOpenAIFinalPromptNonReadToolOmitsCacheGuard(t *testing.T) {
        messages := []any{
                map[string]any{"role": "user", "content": "搜索一下"},
        }
        tools := []any{
                map[string]any{
                        "type": "function",
                        "function": map[string]any{
                                "name":        "search",
                                "description": "Search docs",
                                "parameters": map[string]any{
                                        "type": "object",
                                },
                        },
                },
        }

        finalPrompt, _ := buildOpenAIFinalPrompt(messages, tools, "", false)
        if strings.Contains(finalPrompt, "Read-style cache guard") {
                t.Fatalf("non-read tool prompt should not include read cache guard: %q", finalPrompt)
        }
}

func TestBuildOpenAIFinalPromptWithThinkingKeepsPromptUnchanged(t *testing.T) {
        messages := []any{
                map[string]any{"role": "user", "content": "继续回答上一个问题"},
        }

        finalPromptThinking, _ := buildOpenAIFinalPrompt(messages, nil, "", true)
        finalPromptPlain, _ := buildOpenAIFinalPrompt(messages, nil, "", false)
        if finalPromptThinking != finalPromptPlain {
                t.Fatalf("expected thinking flag not to prepend continuation contract, thinking=%q plain=%q", finalPromptThinking, finalPromptPlain)
        }
}
