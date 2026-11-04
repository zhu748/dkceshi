package claude

import "testing"

type mockClaudeConfig struct {
        aliases map[string]string
}

func (m mockClaudeConfig) ModelAliases() map[string]string { return m.aliases }
func (mockClaudeConfig) CurrentInputFileEnabled() bool     { return true }
func (mockClaudeConfig) CurrentInputFileMinChars() int     { return 0 }
func (mockClaudeConfig) ExpertPromptSegmentEnabled() bool  { return false }
func (mockClaudeConfig) ExpertPromptSegmentMaxChars() int  { return 120000 }

func TestNormalizeClaudeRequestUsesGlobalAliasMapping(t *testing.T) {
        req := map[string]any{
                "model": "claude-opus-4-6",
                "messages": []any{
                        map[string]any{"role": "user", "content": "hello"},
                },
        }
        out, err := normalizeClaudeRequest(mockClaudeConfig{
                aliases: map[string]string{
                        "claude-opus-4-6": "deepseek-v4.1-flash-search",
                },
        }, req)
        if err != nil {
                t.Fatalf("normalizeClaudeRequest error: %v", err)
        }
        if out.Standard.ResolvedModel != "deepseek-v4.1-flash-search" {
                t.Fatalf("resolved model mismatch: got=%q", out.Standard.ResolvedModel)
        }
        if !out.Standard.Thinking || !out.Standard.Search {
                t.Fatalf("unexpected flags: thinking=%v search=%v", out.Standard.Thinking, out.Standard.Search)
        }
}

// 思考开关只由模型名决定：请求里的 thinking 配置（无论开关）仅兼容接收，
// 不影响由模型名解析出的开关。
func TestNormalizeClaudeRequestIgnoresRequestThinkingConfig(t *testing.T) {
        for _, thinkingCfg := range []map[string]any{
                {"type": "disabled"},
                {"type": "enabled", "budget_tokens": 1024},
        } {
                req := map[string]any{
                        "model": "claude-opus-4-6",
                        "messages": []any{
                                map[string]any{"role": "user", "content": "hello"},
                        },
                        "thinking": thinkingCfg,
                }
                out, err := normalizeClaudeRequest(mockClaudeConfig{
                        aliases: map[string]string{
                                "claude-opus-4-6": "deepseek-v4.1-flash",
                        },
                }, req)
                if err != nil {
                        t.Fatalf("normalizeClaudeRequest error: %v", err)
                }
                if !out.Standard.Thinking {
                        t.Fatalf("expected thinking to follow the model name (enabled), got disabled for request thinking=%#v", thinkingCfg)
                }
        }
}

func TestNormalizeClaudeRequestNoThinkingAliasForcesThinkingOff(t *testing.T) {
        req := map[string]any{
                "model": "claude-opus-4-6-nothinking",
                "messages": []any{
                        map[string]any{"role": "user", "content": "hello"},
                },
                "thinking": map[string]any{"type": "enabled", "budget_tokens": 1024},
        }
        out, err := normalizeClaudeRequest(mockClaudeConfig{}, req)
        if err != nil {
                t.Fatalf("normalizeClaudeRequest error: %v", err)
        }
        if out.Standard.ResolvedModel != "deepseek-v4.1-flash-nothinking" {
                t.Fatalf("resolved model mismatch: got=%q", out.Standard.ResolvedModel)
        }
        if out.Standard.Thinking {
                t.Fatalf("expected nothinking alias to force downstream thinking off")
        }
}

func TestNormalizeClaudeRequestPrefersGlobalAliasMapping(t *testing.T) {
        req := map[string]any{
                "model": "claude-sonnet-4-6",
                "messages": []any{
                        map[string]any{"role": "user", "content": "hello"},
                },
        }
        out, err := normalizeClaudeRequest(mockClaudeConfig{
                aliases: map[string]string{
                        "claude-sonnet-4-6": "deepseek-v4.1-flash",
                },
        }, req)
        if err != nil {
                t.Fatalf("normalizeClaudeRequest error: %v", err)
        }
        if out.Standard.ResolvedModel != "deepseek-v4.1-flash" {
                t.Fatalf("expected global alias to win for explicit model, got=%q", out.Standard.ResolvedModel)
        }
}
