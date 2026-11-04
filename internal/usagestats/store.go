package usagestats

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"

	"github.com/google/uuid"
)

const (
	FileVersion   = 3
	FlushInterval = time.Second
)

// gmt8 keeps day bucketing aligned with DeepSeek's official usage page.
var gmt8 = time.FixedZone("GMT+8", 8*60*60)

// ErrInvalidUsageSettings marks user-input validation failures in
// UsageSettings/SaveSettings so HTTP handlers can map them to 400.
var ErrInvalidUsageSettings = errors.New("invalid usage settings")

// Store 是内容无关的 Token 用量账本：按 GMT+8 日期 + 模型 + 调用方 + 账号聚合
// 数值累计与费用估算，仅保存账号的哈希标识和脱敏标签，不保存提示词、回复内容或密钥本体。
// 落盘采用版本化文件 + 后台定时 flush，费用按请求发生时刻的波峰倍率累计。
type Store struct {
	mu              sync.Mutex
	flushMu         sync.Mutex // Serialize disk writes without holding mu during I/O.
	path            string
	settingsPath    string
	entries         map[string]*Entry
	checkpointOrder []string // Immutable keys from the last sorted checkpoint.
	settings        UsageSettings
	instanceID      string
	revision        int64
	dirty           bool
	stop            chan struct{}
	done            chan struct{}
	closeOnce       sync.Once
}

func New(path string) *Store {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	s := &Store{
		path:         filepath.Clean(strings.TrimSpace(path)),
		settingsPath: strings.TrimSpace(path) + ".settings",
		entries:      map[string]*Entry{},
		settings:     DefaultSettings(),
		instanceID:   uuid.NewString(),
	}
	s.mu.Lock()
	if err := s.loadLocked(); err != nil {
		config.Logger.Warn("[usage_stats] unavailable at startup", "path", s.path, "error", err)
		// Quarantine the unreadable ledger instead of silently starting over:
		// with an empty in-memory ledger, the first Record would flush and
		// atomically overwrite the original file — losing the whole history
		// with no backup. Renaming preserves the data for manual recovery.
		if quarantined, qErr := quarantineUnreadableLedger(s.path); qErr != nil {
			config.Logger.Warn("[usage_stats] could not quarantine unreadable ledger", "path", s.path, "error", qErr)
		} else if quarantined != "" {
			config.Logger.Warn("[usage_stats] quarantined unreadable ledger, starting a new one", "original", s.path, "quarantined_as", quarantined)
		}
		s.entries = map[string]*Entry{}
	}
	s.mu.Unlock()
	s.loadSettings()
	s.startFlusher()
	return s
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.done
}
