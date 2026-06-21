package sessionstate

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"ds2api/internal/config"
)

const (
	// MaxEditCount 是 DeepSeek App 允许的最大编辑次数（实测为 6 次）。
	MaxEditCount = 6

	// SessionTTL 是会话状态在本地缓存的默认过期时间。
	// DeepSeek 远端会话在无活动一段时间后也会过期，这里设 30 分钟。
	SessionTTL = 30 * time.Minute

	// CleanupInterval 是后台清理过期条目的间隔。
	CleanupInterval = 5 * time.Minute
)

// State 记录一个远端会话的本地缓存状态，用于 edit_message 复用。
type State struct {
	// SessionID 是 DeepSeek 远端会话 ID。
	SessionID string
	// RequestMessageID 是上一次用户消息的 message_id，
	// 用于 edit_message 请求中的 message_id 字段。
	RequestMessageID int
	// AccountID 是绑定的 DeepSeek 账号标识，确保同一会话始终使用同一账号。
	AccountID string
	// EditCount 是已执行的编辑次数，到达 MaxEditCount 后需要新建会话。
	EditCount int
	// CreatedAt 是状态创建时间。
	CreatedAt time.Time
	// LastAccessAt 是最后一次访问时间，用于 TTL 过期判断。
	LastAccessAt time.Time
}

// Store 是会话状态的内存缓存，支持并发安全访问和 TTL 过期清理。
type Store struct {
	mu    sync.RWMutex
	items map[string]*State // key = fingerprint
}

// NewStore 创建一个新的会话状态缓存。
func NewStore() *Store {
	s := &Store{
		items: make(map[string]*State),
	}
	go s.cleanupLoop()
	return s
}

// ComputeFingerprint 根据 messages 数组的前 N-1 条消息计算指纹。
// 只有前 N-1 条一致，才认为是同一对话的延续（最后一条是新的用户消息）。
// 指纹用于匹配远端已有的会话，实现 edit_message 复用。
func ComputeFingerprint(messages []any) string {
	if len(messages) <= 1 {
		return ""
	}
	// 取前 N-1 条消息
	prefix := messages[:len(messages)-1]
	b, err := json.Marshal(prefix)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}

// Get 根据 fingerprint 查找活跃的会话状态。
// 返回 nil 表示未找到或已过期。
func (s *Store) Get(fingerprint string) *State {
	if fingerprint == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.items[fingerprint]
	if !ok {
		return nil
	}
	if time.Since(state.LastAccessAt) > SessionTTL {
		return nil
	}
	if state.EditCount >= MaxEditCount {
		return nil
	}
	return state
}

// Put 保存或更新会话状态。
func (s *Store) Put(fingerprint string, state *State) {
	if fingerprint == "" || state == nil {
		return
	}
	state.LastAccessAt = time.Now()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[fingerprint] = state
	config.Logger.Debug("[session_state] put",
		"fingerprint", fingerprint[:12]+"...",
		"session_id", state.SessionID,
		"message_id", state.RequestMessageID,
		"account", state.AccountID,
		"edit_count", state.EditCount,
	)
}

// IncrementEditCount 原子地将指定 fingerprint 的 edit_count +1，
// 并更新 RequestMessageID 为新的用户消息 ID。
// 返回更新后的状态，如果 fingerprint 不存在或已过期返回 nil。
func (s *Store) IncrementEditCount(fingerprint string, newRequestMessageID int) *State {
	if fingerprint == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.items[fingerprint]
	if !ok {
		return nil
	}
	if time.Since(state.LastAccessAt) > SessionTTL {
		delete(s.items, fingerprint)
		return nil
	}
	state.EditCount++
	state.RequestMessageID = newRequestMessageID
	state.LastAccessAt = time.Now()
	config.Logger.Debug("[session_state] edit_count incremented",
		"fingerprint", fingerprint[:12]+"...",
		"session_id", state.SessionID,
		"new_message_id", newRequestMessageID,
		"edit_count", state.EditCount,
	)
	return state
}

// Invalidate 使指定 fingerprint 的缓存失效。
// 当 edit_message 返回 edit_limit 或其他错误时调用。
func (s *Store) Invalidate(fingerprint string) {
	if fingerprint == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, fingerprint)
	config.Logger.Debug("[session_state] invalidated", "fingerprint", fingerprint[:12]+"...")
}

// cleanupLoop 定期清理过期条目。
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.cleanup()
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.items {
		if now.Sub(v.LastAccessAt) > SessionTTL {
			delete(s.items, k)
		}
	}
}
