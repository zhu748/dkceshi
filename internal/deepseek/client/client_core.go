package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/devcapture"
	"ds2api/internal/util"
)

// intFrom is a package-internal alias for the shared util version.
var intFrom = util.IntFrom

type Client struct {
	Store      *config.Store
	Auth       *auth.Resolver
	capture    *devcapture.Store
	regular    trans.Doer
	stream     trans.Doer
	fallback   *http.Client
	fallbackS  *http.Client
	maxRetries int

	proxyClientsMu sync.RWMutex
	proxyClients   map[string]requestClients

	// powCache 是 PoW 预取缓存，复刻真实 Android App 在每次 completion 后异步
	// 预取下一个 PoW 的行为，让下一次 completion 几乎零延迟拿到 PoW。
	powCache *powPrefetchCache
}

func NewClient(store *config.Store, resolver *auth.Resolver) *Client {
	c := &Client{
		Store:        store,
		Auth:         resolver,
		capture:      devcapture.Global(),
		regular:      trans.New(60 * time.Second),
		stream:       trans.New(0),
		fallback:     &http.Client{Timeout: 60 * time.Second},
		fallbackS:    &http.Client{Timeout: 0},
		maxRetries:   3,
		proxyClients: map[string]requestClients{},
	}
	// powCache.fetcher 指向同步获取 PoW 的真实函数（即 GetPowForTarget 的同步实现）。
	// 用 closure 形式注入，避免循环依赖。
	c.powCache = newPowPrefetchCache(func(ctx context.Context, a *auth.RequestAuth, targetPath string, maxAttempts int) (string, error) {
		return c.fetchPowSync(ctx, a, targetPath, maxAttempts)
	})
	return c
}

// PreloadPow 保留兼容接口，纯 Go 实现无需预加载。
func (c *Client) PreloadPow(_ context.Context) error {
	return nil
}
