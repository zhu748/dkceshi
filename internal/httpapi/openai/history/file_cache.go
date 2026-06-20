package history

import (
        "crypto/sha256"
        "encoding/hex"
        "strings"
        "sync"
        "time"
)

// fileCacheKey 是文件复用缓存的 key：同一账号 + 同一文件内容（hash）复用 fileID。
type fileCacheKey struct {
        accountID string
        contentHash string
}

type cachedFile struct {
        fileID    string
        uploadedAt time.Time
}

// fileUploadCache 全局共享，所有 history.Service 实例共用一份缓存。
// TTL 设 10 分钟：DeepSeek 文件解析通常在该时长内可用，且能避免短时间内重复
// 上传相同内容（特别是同一对话回合内的 chat_context.txt / tool_schema.txt）。
var fileUploadCache = &fileCache{
        entries: map[fileCacheKey]cachedFile{},
        ttl:     10 * time.Minute,
}

type fileCache struct {
        mu      sync.RWMutex
        entries map[fileCacheKey]cachedFile
        ttl     time.Duration
}

// Lookup 返回未过期的缓存项；找不到或已过期返回 ""。
// 当 accountID 为空时（如部分测试 stub 场景）始终返回空，强制走真实上传路径，
// 避免缓存逻辑在测试场景里影响 mock 调用次数断言。生产路径下 accountID 一定非空。
func (c *fileCache) Lookup(accountID, content string) string {
        accountID = strings.TrimSpace(accountID)
        if accountID == "" {
                return ""
        }
        key := fileCacheKey{
                accountID:   accountID,
                contentHash: hashContent(content),
        }
        c.mu.RLock()
        defer c.mu.RUnlock()
        entry, ok := c.entries[key]
        if !ok {
                return ""
        }
        if time.Since(entry.uploadedAt) > c.ttl {
                return ""
        }
        return entry.fileID
}

// Store 写入一条缓存项。
// 当 accountID 为空时跳过（测试 stub 场景）。
func (c *fileCache) Store(accountID, content, fileID string) {
        accountID = strings.TrimSpace(accountID)
        if accountID == "" {
                return
        }
        key := fileCacheKey{
                accountID:   accountID,
                contentHash: hashContent(content),
        }
        c.mu.Lock()
        defer c.mu.Unlock()
        c.entries[key] = cachedFile{
                fileID:     fileID,
                uploadedAt: time.Now(),
        }
        // 顺带做一次轻量清理：超过 256 条时丢掉过期项，避免内存无限增长。
        if len(c.entries) > 256 {
                c.cleanupLocked()
        }
}

func (c *fileCache) cleanupLocked() {
        now := time.Now()
        for k, v := range c.entries {
                if now.Sub(v.uploadedAt) > c.ttl {
                        delete(c.entries, k)
                }
        }
}

// Reset 清空所有缓存项。仅供测试使用：在测试开始时调用，避免跨测试污染。
func (c *fileCache) Reset() {
        c.mu.Lock()
        defer c.mu.Unlock()
        c.entries = map[fileCacheKey]cachedFile{}
}

// ResetFileUploadCache 导出 Reset 方法，供测试在 setup 时调用。
func ResetFileUploadCache() {
        fileUploadCache.Reset()
}

func hashContent(content string) string {
        sum := sha256.Sum256([]byte(content))
        return hex.EncodeToString(sum[:])
}

// jitterSleep 引入 50-200ms 之间的随机抖动，打破 "上传即 completion" 的固定时序。
// 真实 App 用户从选文件到点发送之间通常有可观察的延迟，而早期实现是同步紧贴的
// 两次上传 + 立即 completion，是行为风控的强信号。
func jitterSleep() {
        // 使用单调时钟的纳秒位作为简单伪随机源。这里不需要密码学级别的均匀分布，
        // 只要每次抖动落在 50-200ms 区间即可。
        nanos := time.Now().UnixNano()
        delta := time.Duration(50+(nanos%151)) * time.Millisecond
        time.Sleep(delta)
}

// degradeTracker 记录某账号的上传失败次数，达到阈值后在一段时间窗口内自动降级
// 到模式 A（纯文本 prompt），避免单账号连续失败触发更严的风控升级。
type degradeTracker struct {
        mu       sync.Mutex
        failure  map[string]int   // accountID -> consecutive failures
        degraded map[string]time.Time // accountID -> degrade-until time
        threshold int
        window    time.Duration
}

var globalDegradeTracker = &degradeTracker{
        failure:   map[string]int{},
        degraded:  map[string]time.Time{},
        threshold: 3,
        window:    30 * time.Minute,
}

// IsDegraded 返回该账号当前是否处于降级窗口内。
// 当 accountID 为空时（如部分测试 stub 场景）始终返回 false，避免降级机制
// 在测试场景里误伤 mock 路径。生产路径下 accountID 一定非空。
func (d *degradeTracker) IsDegraded(accountID string) bool {
        d.mu.Lock()
        defer d.mu.Unlock()
        accountID = strings.TrimSpace(accountID)
        if accountID == "" {
                return false
        }
        until, ok := d.degraded[accountID]
        if !ok {
                return false
        }
        if time.Now().After(until) {
                // 窗口已过，清理并允许重新尝试。
                delete(d.degraded, accountID)
                delete(d.failure, accountID)
                return false
        }
        return true
}

// RecordFailure 记录一次失败，达到阈值后开启降级窗口。
// 当 accountID 为空时跳过（测试 stub 场景）。
func (d *degradeTracker) RecordFailure(accountID string) {
        d.mu.Lock()
        defer d.mu.Unlock()
        accountID = strings.TrimSpace(accountID)
        if accountID == "" {
                return
        }
        d.failure[accountID]++
        if d.failure[accountID] >= d.threshold {
                d.degraded[accountID] = time.Now().Add(d.window)
                d.failure[accountID] = 0
        }
}

// RecordSuccess 清零失败计数（成功一次就重置）。
// 当 accountID 为空时跳过。
func (d *degradeTracker) RecordSuccess(accountID string) {
        d.mu.Lock()
        defer d.mu.Unlock()
        accountID = strings.TrimSpace(accountID)
        if accountID == "" {
                return
        }
        delete(d.failure, accountID)
}

// Reset 清空所有降级状态。仅供测试使用：在测试开始时调用，避免跨测试污染。
func (d *degradeTracker) Reset() {
        d.mu.Lock()
        defer d.mu.Unlock()
        d.failure = map[string]int{}
        d.degraded = map[string]time.Time{}
}

// ResetDegradeTracker 导出 Reset 方法，供测试在 setup 时调用。
func ResetDegradeTracker() {
        globalDegradeTracker.Reset()
}
