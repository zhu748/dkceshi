package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"ds2api/internal/auth"
)

// powPrefetchCache 实现按账号 + target_path 维度的 PoW 预取缓存。
//
// 真实 Android App 在每次 completion 发出后立即异步请求下一个 PoW，
// 让下一次 completion 几乎零延迟拿到 PoW。本 cache 复刻该行为：
//
//  1. GetPow 优先返回缓存中未过期的 PoW；缓存命中时立刻清空并异步触发下一次预取
//  2. 缓存 miss 时同步获取
//  3. CallCompletion 成功后调用 SchedulePrefetch 异步预取下一个 PoW
//
// 线程安全。所有方法可在多 goroutine 并发调用。
type powPrefetchCache struct {
	mu      sync.Mutex
	entries map[string]*powCacheEntry
	// fetcher 是同步获取 PoW 的真实函数（由 Client 注入），返回的 PoW 字符串
	// 即可直接作为 x-ds-pow-response header 值。
	fetcher func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error)
	// now 用于注入测试用时钟；生产环境返回 time.Now()。
	now func() time.Time
}

type powCacheEntry struct {
	// pow 是已计算好的 PoW header 值（base64 JSON）
	pow string
	// fetchedAt 是 PoW 获取时间
	fetchedAt time.Time
	// expireAt 是 PoW 在服务端的过期时间（从 challenge.expire_at 解析）
	expireAt time.Time
	// fetching 标记当前是否正在异步预取，避免重复预取
	fetching bool
}

// powSafetyMargin 是 PoW 使用前的预留安全时间。
// 真实 App 在过期前 ~10s 就会刷新；这里保守取 30s，避免 completion 慢请求时 PoW 已过期。
const powSafetyMargin = 30 * time.Second

// powPrefetchTimeout 是异步预取请求的最大耗时，超时则放弃，下次 GetPow 会触发同步获取。
const powPrefetchTimeout = 10 * time.Second

func newPowPrefetchCache(fetcher func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error)) *powPrefetchCache {
	return &powPrefetchCache{
		entries: make(map[string]*powCacheEntry),
		fetcher: fetcher,
		now:     time.Now,
	}
}

// cacheKey 由账号 ID + target path 组合，区分不同 endpoint 的 PoW（completion vs upload）。
func powCacheKey(accountID, targetPath string) string {
	return accountID + "|" + targetPath
}

// Get 从缓存取 PoW；缓存命中且未过期（含安全余量）时返回 true。
// 命中后立刻清空条目，并异步触发下一次预取，复刻 App 行为。
// 未命中时返回 ("", false)，调用方需走同步获取路径。
func (c *powPrefetchCache) Get(accountID, targetPath string) (string, bool) {
	if c == nil {
		return "", false
	}
	key := powCacheKey(accountID, targetPath)
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok || entry.pow == "" {
		c.mu.Unlock()
		return "", false
	}
	// 检查是否过期（含安全余量）
	if c.now().After(entry.expireAt.Add(-powSafetyMargin)) {
		delete(c.entries, key)
		c.mu.Unlock()
		return "", false
	}
	// 命中，取出并清空（PoW 是一次性的）
	pow := entry.pow
	delete(c.entries, key)
	c.mu.Unlock()
	return pow, true
}

// Put 把同步获取到的 PoW 写入缓存，便于下次 GetPow 命中。
// expireAt 应来自 challenge.expire_at；为 0 时按 5 分钟默认 TTL 处理。
func (c *powPrefetchCache) Put(accountID, targetPath, pow string, expireAt time.Time) {
	if c == nil || pow == "" {
		return
	}
	key := powCacheKey(accountID, targetPath)
	if expireAt.IsZero() {
		expireAt = c.now().Add(5 * time.Minute)
	}
	c.mu.Lock()
	c.entries[key] = &powCacheEntry{
		pow:       pow,
		fetchedAt: c.now(),
		expireAt:  expireAt,
	}
	c.mu.Unlock()
}

// SchedulePrefetch 异步预取一个 PoW 写入缓存。如果当前已有缓存或正在预取，则跳过。
// 调用方应在每次 CallCompletion 成功后调用，复刻 App 在 completion 后立刻预取的行为。
// ctx 应是 long-lived 的 context（不要传请求 context，否则请求结束预取就被取消）。
func (c *powPrefetchCache) SchedulePrefetch(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) {
	if c == nil || c.fetcher == nil || a == nil {
		return
	}
	key := powCacheKey(a.AccountID, targetPath)
	c.mu.Lock()
	// 已有缓存或正在预取，跳过
	if existing, ok := c.entries[key]; ok && existing.pow != "" {
		c.mu.Unlock()
		return
	}
	if existing, ok := c.entries[key]; ok && existing.fetching {
		c.mu.Unlock()
		return
	}
	// 标记正在预取
	c.entries[key] = &powCacheEntry{fetching: true}
	c.mu.Unlock()

	go func() {
		prefetchCtx, cancel := context.WithTimeout(context.Background(), powPrefetchTimeout)
		defer cancel()
		// 复用原始 ctx 的认证信息（auth.FromContext），但用独立 timeout context 避免被父 ctx 取消
		prefetchCtx = auth.WithAuth(prefetchCtx, a)
		pow, err := c.fetcher(prefetchCtx, a, targetPath, maxAttempts)
		c.mu.Lock()
		if err != nil || pow == "" {
			// 预取失败：删除"正在预取"标记，下次 GetPow 会走同步获取
			delete(c.entries, key)
			c.mu.Unlock()
			return
		}
		// 写入预取结果（按 5 分钟默认 TTL；fetcher 内部如有 expire_at 可后续扩展覆盖）
		c.entries[key] = &powCacheEntry{
			pow:       pow,
			fetchedAt: c.now(),
			expireAt:  c.now().Add(5 * time.Minute),
		}
		c.mu.Unlock()
	}()
}

// Clear 清空所有缓存条目。账号登出/切换时可调用。
func (c *powPrefetchCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]*powCacheEntry)
	c.mu.Unlock()
}

// ClearForAccount 清空指定账号的所有缓存条目。
func (c *powPrefetchCache) ClearForAccount(accountID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	for k := range c.entries {
		if len(k) > len(accountID)+1 && k[:len(accountID)] == accountID {
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()
}

// Size 返回当前缓存条目数量（主要用于测试）。
func (c *powPrefetchCache) Size() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// errPoWCacheMiss 是内部 sentinel，用于 GetPow 内部判断是否需要 fallback 到同步获取。
var errPoWCacheMiss = errors.New("pow cache miss")
