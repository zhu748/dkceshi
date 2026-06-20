package prompt

import (
        "strings"
        "testing"
)

func TestNormalizeContentNilReturnsEmpty(t *testing.T) {
        if got := NormalizeContent(nil); got != "" {
                t.Fatalf("expected empty string for nil content, got %q", got)
        }
}

func TestMessagesPrepareNilContentNoNullLiteral(t *testing.T) {
        messages := []map[string]any{
                {"role": "assistant", "content": nil},
                {"role": "user", "content": "ok"},
        }
        got := MessagesPrepare(messages)
        if got == "" {
                t.Fatalf("expected non-empty output")
        }
        if got == "null" {
                t.Fatalf("expected no null literal output, got %q", got)
        }
}

func TestMessagesPrepareUsesTurnSuffixes(t *testing.T) {
        messages := []map[string]any{
                {"role": "system", "content": "System rule"},
                {"role": "user", "content": "Question"},
                {"role": "assistant", "content": "Answer"},
        }
        got := MessagesPrepare(messages)
        if !strings.HasPrefix(got, "<|begin▁of▁sentence|>") {
                t.Fatalf("expected begin-of-sentence marker, got %q", got)
        }
        if !strings.Contains(got, "<|System|>") || !strings.Contains(got, "<|end▁of▁instructions|>") || !strings.Contains(got, "System rule") {
                t.Fatalf("expected system instructions to remain present, got %q", got)
        }
        if !strings.Contains(got, "<|User|>Question") {
                t.Fatalf("expected user question, got %q", got)
        }
        if !strings.Contains(got, "<|Assistant|>Answer<|end▁of▁sentence|>") {
                t.Fatalf("expected assistant sentence suffix, got %q", got)
        }
        if strings.Contains(got, "<think>") || strings.Contains(got, "</think>") {
                t.Fatalf("did not expect think tags in prompt, got %q", got)
        }
}

func TestMessagesPreparePrependsOutputIntegrityGuard(t *testing.T) {
        // Output integrity guard 默认开启。该测试验证：
        // 1) 默认情况下 final prompt 包含 guard
        // 2) 通过 SetOutputIntegrityGuardEnabled(false) 显式关闭后，guard 不再注入
        messages := []map[string]any{
                {"role": "system", "content": "System rule"},
                {"role": "user", "content": "Question"},
        }

        // 默认开启
        got := MessagesPrepare(messages)
        if !strings.HasPrefix(got, beginSentenceMarker+systemMarker+outputIntegrityGuardPrompt) {
                t.Fatalf("expected output integrity guard to be prepended by default, got %q", got)
        }
        if !strings.Contains(got, outputIntegrityGuardPrompt+"\n\nSystem rule") {
                t.Fatalf("expected output integrity guard to precede system prompt content, got %q", got)
        }

        // 显式关闭
        SetOutputIntegrityGuardEnabled(false)
        defer SetOutputIntegrityGuardEnabled(true)
        got = MessagesPrepare(messages)
        if strings.Contains(got, outputIntegrityGuardPrompt) {
                t.Fatalf("output integrity guard should be removed when disabled, got %q", got)
        }
        if !strings.Contains(got, "<|User|>Question") {
                t.Fatalf("expected user question to remain present, got %q", got)
        }
}

func TestNormalizeContentArrayFallsBackToContentWhenTextEmpty(t *testing.T) {
        got := NormalizeContent([]any{
                map[string]any{"type": "text", "text": "", "content": "from-content"},
        })
        if got != "from-content" {
                t.Fatalf("expected fallback to content when text is empty, got %q", got)
        }
}

func TestMessagesPrepareWithThinkingPreservesPromptShape(t *testing.T) {
        messages := []map[string]any{{"role": "user", "content": "Question"}}
        gotThinking := MessagesPrepareWithThinking(messages, true)
        gotPlain := MessagesPrepareWithThinking(messages, false)
        if gotThinking != gotPlain {
                t.Fatalf("expected thinking flag not to add extra continuity instructions, got thinking=%q plain=%q", gotThinking, gotPlain)
        }
        if !strings.HasSuffix(gotThinking, "<|Assistant|>") {
                t.Fatalf("expected assistant suffix, got %q", gotThinking)
        }
}

// TestMessagesPrepareWithToolHintSkipsGuardForPureNoToolChat 验证：
// 本次请求无工具（hasTools=false）且历史不含 tool/function 角色消息时，
// Output integrity guard 应被跳过，避免多余的 system 消息成为指纹。
func TestMessagesPrepareWithToolHintSkipsGuardForPureNoToolChat(t *testing.T) {
        messages := []map[string]any{
                {"role": "user", "content": "The attached file holds the earlier conversation. Read it and respond to the most recent user request directly."},
        }
        got := MessagesPrepareWithThinkingAndToolHint(messages, false, false)
        if strings.Contains(got, outputIntegrityGuardMarker) {
                t.Fatalf("expected output integrity guard to be skipped for pure no-tool chat, got %q", got)
        }
        if !strings.Contains(got, "<|User|>") || !strings.HasSuffix(got, "<|Assistant|>") {
                t.Fatalf("expected bare user→assistant prompt shape, got %q", got)
        }
}

// TestMessagesPrepareWithToolHintInjectsGuardWhenToolsPresent 验证：
// 本次请求有工具（hasTools=true）时，即使 messages 里没有 tool/function 角色，
// Output integrity guard 仍应被注入，防止模型回显 DSML 解析残留。
func TestMessagesPrepareWithToolHintInjectsGuardWhenToolsPresent(t *testing.T) {
        messages := []map[string]any{
                {"role": "system", "content": "You are helpful"},
                {"role": "user", "content": "请调用工具"},
        }
        got := MessagesPrepareWithThinkingAndToolHint(messages, false, true)
        if !strings.Contains(got, outputIntegrityGuardMarker) {
                t.Fatalf("expected output integrity guard to be injected when tools are present, got %q", got)
        }
}

// TestMessagesPrepareWithToolHintInjectsGuardWhenHistoryHasToolMessages 验证：
// 本次请求无工具，但历史含 tool/function 角色消息时，guard 仍应注入，
// 防止模型回显历史 tool 输出乱码。
func TestMessagesPrepareWithToolHintInjectsGuardWhenHistoryHasToolMessages(t *testing.T) {
        messages := []map[string]any{
                {"role": "user", "content": "请查询天气"},
                {"role": "assistant", "content": "<|DSML|tool_calls>\n  <|DSML|invoke name=\"get_weather\">\n    <|DSML|parameter name=\"city\"><![CDATA[北京]]></|DSML|parameter>\n  </|DSML|invoke>\n</|DSML|tool_calls>"},
                {"role": "tool", "content": "{\"weather\":\"sunny\"}"},
                {"role": "user", "content": "谢谢"},
        }
        got := MessagesPrepareWithThinkingAndToolHint(messages, false, false)
        if !strings.Contains(got, outputIntegrityGuardMarker) {
                t.Fatalf("expected output integrity guard to be injected when history contains tool messages, got %q", got)
        }
}

// TestMessagesPrepareWithToolHintInjectsGuardWhenAssistantHasDSMLHistory 验证：
// 即使没有独立的 tool 角色消息，只要 assistant 历史里渲染过 DSML tool_calls 块，
// 就认为上下文含工具调用残留，guard 应注入。
func TestMessagesPrepareWithToolHintInjectsGuardWhenAssistantHasDSMLHistory(t *testing.T) {
        messages := []map[string]any{
                {"role": "user", "content": "请查询"},
                {"role": "assistant", "content": "<|DSML|tool_calls>\n  <|DSML|invoke name=\"search\">\n  </|DSML|invoke>\n</|DSML|tool_calls>"},
                {"role": "user", "content": "结果呢？"},
        }
        got := MessagesPrepareWithThinkingAndToolHint(messages, false, false)
        if !strings.Contains(got, outputIntegrityGuardMarker) {
                t.Fatalf("expected output integrity guard when assistant history contains DSML tool_calls, got %q", got)
        }
}
