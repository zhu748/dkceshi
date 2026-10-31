package account

import (
	"sort"
	"sync"
	"time"

	"ds2api/internal/config"
)

type Pool struct {
	store *config.Store
	mu    sync.Mutex
	queue []string
	inUse map[string]int
	// inUseTotal mirrors sum(inUse.values()) and is maintained on every
	// acquire/release so the global-limit check stays O(1) on the hot path.
	inUseTotal             int
	waiters                []*poolWaiter
	maxInflightPerAccount  int
	recommendedConcurrency int
	maxQueueSize           int
	globalMaxInflight      int
	rateLimits             map[string]*rateLimitEntry
	rateLimitWindow        time.Duration
	// 24h 速率限制（见 pool_daily_limit.go）：当天每账号计数、该计数属于哪一天
	// （GMT+8 自然日）、计数文件路径、上次落盘时刻、落盘间隔与待执行的补偿写盘。
	dailyCounts          map[string]int
	dailyDate            string
	dailyPath            string
	dailyLastPersist     time.Time
	dailyPersistInterval time.Duration
	dailyFlushTimer      *time.Timer
	expiryTimer          *time.Timer
	closed               bool
	onStateChanged       func()
}

func NewPool(store *config.Store) *Pool {
	maxPer := 2
	if store != nil {
		maxPer = store.RuntimeAccountMaxInflight()
	}
	p := &Pool{
		store:                 store,
		inUse:                 map[string]int{},
		maxInflightPerAccount: maxPer,
		rateLimits:            make(map[string]*rateLimitEntry),
		dailyCounts:           map[string]int{},
		dailyPath:             config.AccountDailyLimitPath(),
	}
	if store != nil && store.Snapshot().ElasticPool.Enabled {
		if err := store.Update(func(c *config.Config) error { ReconcileElasticPool(c); return nil }); err != nil {
			config.Logger.Warn("[account_pool] initial reconciliation failed", "error", err)
		}
	}
	p.Reset()
	// 启动检查：计数文件里的日期不是今天就清零，是今天就接着用。
	p.startupDailyLimitCheck()
	return p
}

func (p *Pool) Reset() {
	accounts := p.store.Accounts()
	sort.SliceStable(accounts, func(i, j int) bool {
		iHas := accounts[i].Token != ""
		jHas := accounts[j].Token != ""
		if iHas == jHas {
			return i < j
		}
		return iHas
	})
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		// 禁用/被封/鉴权异常账号不进入取号队列：它们不会被调度，留在队列里
		// 只会让每次取号退化为对全量账号（含大量休眠账号）的线性扫描，
		// 推高 CPU 占用。
		if a.Disabled || a.Banned || a.IsAuthFailed() {
			continue
		}
		id := a.Identifier()
		if id != "" {
			ids = append(ids, id)
		}
	}
	maxPer := maxInflightFromEnv()
	if p.store != nil {
		maxPer = p.store.RuntimeAccountMaxInflight()
	}
	recommended := defaultRecommendedConcurrency(len(ids), maxPer)
	queueLimit := maxQueueFromEnv(recommended)
	globalLimit := recommended
	if p.store != nil {
		queueLimit = p.store.RuntimeAccountMaxQueue(recommended)
		globalLimit = p.store.RuntimeGlobalMaxInflight(recommended)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.maxInflightPerAccount = maxPer
	p.drainWaitersLocked()
	p.queue = ids
	// Keep accounting for every configured account, including sleeping accounts
	// and requests still running while an administrator disables an account.
	configured := make(map[string]bool, len(accounts))
	for _, acc := range accounts {
		configured[acc.Identifier()] = true
	}
	for id, count := range p.inUse {
		if !configured[id] {
			delete(p.inUse, id)
			p.inUseTotal -= count
		}
	}
	for id, entry := range p.rateLimits {
		if !configured[id] {
			if entry.timer != nil {
				entry.timer.Stop()
			}
			delete(p.rateLimits, id)
		}
	}
	p.pruneDailyCountsLocked(configured)
	p.scheduleExpiryLocked(accounts)
	p.recommendedConcurrency = recommended
	p.maxQueueSize = queueLimit
	p.globalMaxInflight = globalLimit
	config.Logger.Info(
		"[init_account_queue] initialized",
		"total", len(ids),
		"max_inflight_per_account", p.maxInflightPerAccount,
		"global_max_inflight", p.globalMaxInflight,
		"recommended_concurrency", p.recommendedConcurrency,
		"max_queue_size", p.maxQueueSize,
	)
}

func (p *Pool) Release(accountID string) {
	if accountID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	count := p.inUse[accountID]
	if count <= 0 {
		return
	}
	p.inUseTotal--
	if count == 1 {
		delete(p.inUse, accountID)
		p.notifyWaiterLocked()
		return
	}
	p.inUse[accountID] = count - 1
	p.notifyWaiterLocked()
}

// SchedulableCount 返回取号队列中的可调度账号数（已排除禁用/封禁账号）。
func (p *Pool) SchedulableCount() int {
	return p.SchedulableCountMatching(nil)
}

// SchedulableCountMatching 返回满足 filter 的可调度账号数。
func (p *Pool) SchedulableCountMatching(filter AccountFilter) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, id := range p.queue {
		acc, ok := p.store.FindAccount(id)
		if !ok || !acc.IsSchedulable() {
			continue
		}
		if filter == nil || filter(acc) {
			count++
		}
	}
	return count
}

// GlobalMaxInflight 返回当前号池生效的全局最大在途并发限制（0 表示不限制）。
func (p *Pool) GlobalMaxInflight() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.globalMaxInflight
}

func (p *Pool) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	available := make([]string, 0, len(p.queue))
	inUseAccounts := make([]string, 0, len(p.inUse))
	rateLimited := make([]string, 0)
	dailyLimited := make([]string, 0)
	inUseSlots := 0
	for _, id := range p.queue {
		acc, ok := p.store.FindAccount(id)
		if !ok || !acc.IsSchedulable() {
			continue
		}
		if p.isRateLimitedLocked(id) {
			rateLimited = append(rateLimited, id)
			continue
		}
		if p.isDailyLimitedLocked(id) {
			dailyLimited = append(dailyLimited, id)
			continue
		}
		if p.inUse[id] < p.maxInflightPerAccount {
			available = append(available, id)
		}
	}
	for id, count := range p.inUse {
		if count > 0 {
			inUseAccounts = append(inUseAccounts, id)
			inUseSlots += count
		}
	}
	sort.Strings(inUseAccounts)
	sort.Strings(rateLimited)
	sort.Strings(dailyLimited)
	return map[string]any{
		"available":                len(available),
		"in_use":                   inUseSlots,
		"total":                    len(p.store.Accounts()),
		"available_accounts":       available,
		"in_use_accounts":          inUseAccounts,
		"rate_limited_accounts":    rateLimited,
		"daily_limited_accounts":   dailyLimited,
		"daily_limit_date":         p.dailyLimitDateLocked(),
		"daily_limit_max_per_day":  p.dailyLimitMaxPerDayLocked(),
		"max_inflight_per_account": p.maxInflightPerAccount,
		"global_max_inflight":      p.globalMaxInflight,
		"recommended_concurrency":  p.recommendedConcurrency,
		"waiting":                  len(p.waiters),
		"max_queue_size":           p.maxQueueSize,
	}
}
