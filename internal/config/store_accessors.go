package config

import (
        "os"
        "strconv"
        "strings"
)

func (s *Store) ModelAliases() map[string]string {
        s.mu.RLock()
        defer s.mu.RUnlock()
        out := DefaultModelAliases()
        for k, v := range s.cfg.ModelAliases {
                key := strings.TrimSpace(lower(k))
                val := strings.TrimSpace(lower(v))
                if key == "" || val == "" {
                        continue
                }
                out[key] = val
        }
        return out
}

func (s *Store) ToolcallMode() string {
        return "feature_match"
}

func (s *Store) ToolcallEarlyEmitConfidence() string {
        return "high"
}

func (s *Store) ResponsesStoreTTLSeconds() int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Responses.StoreTTLSeconds > 0 {
                return s.cfg.Responses.StoreTTLSeconds
        }
        return 900
}

func (s *Store) EmbeddingsProvider() string {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return strings.TrimSpace(s.cfg.Embeddings.Provider)
}

func (s *Store) AutoDeleteMode() string {
        s.mu.RLock()
        defer s.mu.RUnlock()
        mode := strings.ToLower(strings.TrimSpace(s.cfg.AutoDelete.Mode))
        switch mode {
        case "none", "single", "all":
                return mode
        }
        if s.cfg.AutoDelete.Sessions {
                return "all"
        }
        return "none"
}

func (s *Store) AdminPasswordHash() string {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return strings.TrimSpace(s.cfg.Admin.PasswordHash)
}

func (s *Store) AdminJWTExpireHours() int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Admin.JWTExpireHours > 0 {
                return s.cfg.Admin.JWTExpireHours
        }
        if raw := strings.TrimSpace(os.Getenv("DS2API_JWT_EXPIRE_HOURS")); raw != "" {
                if n, err := strconv.Atoi(raw); err == nil && n > 0 {
                        return n
                }
        }
        return 24
}

func (s *Store) AdminJWTValidAfterUnix() int64 {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.cfg.Admin.JWTValidAfterUnix
}

func (s *Store) RuntimeAccountMaxInflight() int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Runtime.AccountMaxInflight > 0 {
                return s.cfg.Runtime.AccountMaxInflight
        }
        if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_INFLIGHT")); raw != "" {
                if n, err := strconv.Atoi(raw); err == nil && n > 0 {
                        return n
                }
        }
        return 2
}

func (s *Store) RuntimeAccountMaxQueue(defaultSize int) int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Runtime.AccountMaxQueue > 0 {
                return s.cfg.Runtime.AccountMaxQueue
        }
        if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_QUEUE")); raw != "" {
                if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
                        return n
                }
        }
        if defaultSize < 0 {
                return 0
        }
        return defaultSize
}

func (s *Store) RuntimeGlobalMaxInflight(defaultSize int) int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Runtime.GlobalMaxInflight > 0 {
                return s.cfg.Runtime.GlobalMaxInflight
        }
        if raw := strings.TrimSpace(os.Getenv("DS2API_GLOBAL_MAX_INFLIGHT")); raw != "" {
                if n, err := strconv.Atoi(raw); err == nil && n > 0 {
                        return n
                }
        }
        if defaultSize < 0 {
                return 0
        }
        return defaultSize
}

func (s *Store) RuntimeTokenRefreshIntervalHours() int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.Runtime.TokenRefreshIntervalHours > 0 {
                return s.cfg.Runtime.TokenRefreshIntervalHours
        }
        return 6
}

func (s *Store) AutoDeleteSessions() bool {
        return s.AutoDeleteMode() != "none"
}

func (s *Store) CurrentInputFileEnabled() bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        // 默认关闭：历史拆分（current_input_file）作为高级功能，需要用户显式开启，
        // 或通过 -forcehistory 模型后缀按需触发，避免新部署实例默认就走文件上传路径。
        if s.cfg.CurrentInputFile.Enabled == nil {
                return false
        }
        return *s.cfg.CurrentInputFile.Enabled
}

func (s *Store) CurrentInputFileMinChars() int {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.cfg.CurrentInputFile.MinChars
}

func (s *Store) ThinkingInjectionEnabled() bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.ThinkingInjection.Enabled == nil {
                return true
        }
        return *s.cfg.ThinkingInjection.Enabled
}

func (s *Store) ThinkingInjectionPrompt() string {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return strings.TrimSpace(s.cfg.ThinkingInjection.Prompt)
}

// OutputIntegrityGuardEnabled 控制是否在 prompt 前置注入 "Output integrity guard:"
// 系统消息。默认开启（返回 true）：该 guard 用于防止模型把上游乱码/重复片段
// 当成自己的输出回显，是质量保障的关键环节，宁可带轻度指纹也要保留。
// 若需要极致降低指纹可通过 config 显式设置为 false 关闭。
func (s *Store) OutputIntegrityGuardEnabled() bool {
        s.mu.RLock()
        defer s.mu.RUnlock()
        if s.cfg.OutputIntegrityGuard.Enabled == nil {
                return true
        }
        return *s.cfg.OutputIntegrityGuard.Enabled
}
