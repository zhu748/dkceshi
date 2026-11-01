package keepalive

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/sse"
)

var defaultPrompts = []string{
	"现在纽约天气如何？",
	"现在几点了？",
	"你好啊",
	"简单介绍一下你自己",
	"9.8-9.11=？",
}

// ParseIntervalRange parses an interval string (hours) such as "2-4", "2 - 4", "3", or "3.5".
// Returns min and max duration in time.Duration. If invalid, falls back to default 2h-4h.
func ParseIntervalRange(s string) (minDur, maxDur time.Duration) {
	defaultMin := 2 * time.Hour
	defaultMax := 4 * time.Hour

	raw := strings.TrimSpace(strings.ToLower(s))
	raw = strings.TrimSuffix(raw, "h")
	raw = strings.TrimSuffix(raw, "hours")
	raw = strings.TrimSuffix(raw, "hour")
	raw = strings.TrimSpace(raw)

	if raw == "" {
		return defaultMin, defaultMax
	}

	const minHours = 0.05
	const maxHours = 720.0

	if parts := strings.Split(raw, "-"); len(parts) == 2 {
		minStr := strings.TrimSpace(parts[0])
		maxStr := strings.TrimSpace(parts[1])
		minVal, err1 := strconv.ParseFloat(minStr, 64)
		maxVal, err2 := strconv.ParseFloat(maxStr, 64)
		if err1 != nil || err2 != nil || minVal <= 0 || maxVal <= 0 {
			return defaultMin, defaultMax
		}
		if minVal > maxVal {
			minVal, maxVal = maxVal, minVal
		}
		if minVal < minHours {
			minVal = minHours
		}
		if maxVal > maxHours {
			maxVal = maxHours
		}
		return time.Duration(minVal * float64(time.Hour)), time.Duration(maxVal * float64(time.Hour))
	}

	val, err := strconv.ParseFloat(raw, 64)
	if err != nil || val <= 0 {
		return defaultMin, defaultMax
	}
	if val < minHours {
		val = minHours
	}
	if val > maxHours {
		val = maxHours
	}
	d := time.Duration(val * float64(time.Hour))
	return d, d
}

// RandomDurationFromInterval returns a random duration between the configured interval range.
func RandomDurationFromInterval(intervalStr string) time.Duration {
	minDur, maxDur := ParseIntervalRange(intervalStr)
	if minDur == maxDur || maxDur <= minDur {
		if minDur < time.Second {
			return time.Second
		}
		return minDur
	}
	diff := maxDur - minDur
	rnd := time.Duration(rand.Int63n(int64(diff)))
	dur := minDur + rnd
	if dur < time.Second {
		return time.Second
	}
	return dur
}

// SplitPrompts splits a prompt configuration string by English or Chinese semicolons.
func SplitPrompts(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ';' || r == '；'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// PickPrompt picks a prompt randomly from the configured semicolon-separated string.
// If empty, falls back to defaultPrompts.
func PickPrompt(s string) string {
	prompts := SplitPrompts(s)
	if len(prompts) == 0 {
		prompts = defaultPrompts
	}
	idx := rand.Intn(len(prompts))
	return prompts[idx]
}

type accountTimer struct {
	active     bool
	timer      *time.Timer
	retryCount int
}

// DeepSeekCaller abstracts the subset of client methods used for keep-alive pings.
type DeepSeekCaller interface {
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error)
	DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error)
}

// Manager coordinates demand-driven account keep-alive pings.
type Manager struct {
	store       *config.Store
	pool        *account.Pool
	ds          DeepSeekCaller
	resolver    *auth.Resolver
	chatHistory *chathistory.Store

	mu      sync.Mutex
	timers  map[string]*accountTimer
	stopped bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// afterFunc is injectable for deterministic testing.
	afterFunc func(d time.Duration, f func()) *time.Timer
}

// NewManager creates a new keep-alive manager.
func NewManager(store *config.Store, pool *account.Pool, ds DeepSeekCaller, resolver *auth.Resolver, chatHistory *chathistory.Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:       store,
		pool:        pool,
		ds:          ds,
		resolver:    resolver,
		chatHistory: chatHistory,
		timers:      make(map[string]*accountTimer),
		ctx:         ctx,
		cancel:      cancel,
		afterFunc:   time.AfterFunc,
	}
}

// OnAccountRequest is called when an account handles an incoming request.
// It starts a timer if the account is idle, and ignores subsequent requests during the timer window.
func (m *Manager) OnAccountRequest(accountID string) {
	if m == nil || m.store == nil || !m.store.KeepAliveEnabled() {
		return
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}

	entry, ok := m.timers[accountID]
	if !ok {
		entry = &accountTimer{}
		m.timers[accountID] = entry
	}

	// 期间再有请求不管（不重新计时、不延长）
	if entry.active {
		return
	}

	entry.active = true
	entry.retryCount = 0
	dur := RandomDurationFromInterval(m.store.KeepAliveInterval())
	entry.timer = m.afterFunc(dur, func() {
		m.triggerKeepAlive(accountID)
	})
	config.Logger.Debug("[keepalive] timer scheduled", "account", accountID, "duration", dur)
}

// triggerKeepAlive handles timer expiration, performs the keep-alive request, and resets the timer state.
func (m *Manager) triggerKeepAlive(accountID string) {
	defer func() {
		if r := recover(); r != nil {
			config.Logger.Error("[keepalive] recovered from panic in triggerKeepAlive", "account", accountID, "panic", r)
		}
	}()

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()

	if m.store == nil || !m.store.KeepAliveEnabled() {
		m.resetAccountTimer(accountID)
		return
	}

	acc, ok := m.store.FindAccount(accountID)
	if !ok {
		config.Logger.Debug("[keepalive] account missing, forgotten", "account", accountID)
		m.ForgetAccount(accountID)
		return
	}
	if !acc.IsSchedulable() {
		config.Logger.Debug("[keepalive] account not schedulable, skipped", "account", accountID)
		m.resetAccountTimer(accountID)
		return
	}

	// 占用号池槽位（仅占并发，不计入 24h 请求配额，不触发每分钟速率限制）
	if m.pool != nil {
		pooledAcc, acquired := m.pool.AcquireKeepAlive(accountID)
		if !acquired {
			// 若当前正忙，安排短暂重试（最多 2 次），避免一次冲突导致永久丢失保活
			m.mu.Lock()
			entry := m.timers[accountID]
			if entry != nil && entry.active && entry.retryCount < 2 && !m.stopped {
				entry.retryCount++
				retryDelay := 1 * time.Minute
				entry.timer = m.afterFunc(retryDelay, func() {
					m.triggerKeepAlive(accountID)
				})
				m.mu.Unlock()
				config.Logger.Debug("[keepalive] account busy, scheduled retry", "account", accountID, "attempt", entry.retryCount)
				return
			}
			m.mu.Unlock()

			config.Logger.Debug("[keepalive] account busy and retry exhausted, skipped", "account", accountID)
			m.resetAccountTimer(accountID)
			return
		}
		defer m.pool.Release(accountID)
		acc = pooledAcc
	}

	// 保活完成后重置计时状态
	defer m.resetAccountTimer(accountID)

	ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
	defer cancel()

	m.executeKeepAlive(ctx, acc)
}

func (m *Manager) resetAccountTimer(accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.timers[accountID]; ok {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		entry.active = false
		entry.timer = nil
		entry.retryCount = 0
	}
}

// ForgetAccount stops and removes any timer associated with the given account.
func (m *Manager) ForgetAccount(accountID string) {
	if m == nil {
		return
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.timers[accountID]; ok {
		if entry.timer != nil {
			entry.timer.Stop()
		}
		delete(m.timers, accountID)
	}
}

// executeKeepAlive runs the keep-alive round for a single account.
func (m *Manager) executeKeepAlive(ctx context.Context, acc config.Account) {
	promptText := PickPrompt(m.store.KeepAlivePrompts())

	var a *auth.RequestAuth
	if m.resolver != nil {
		a = m.resolver.NewKeepAliveAuth(acc)
	} else {
		id := acc.Identifier()
		a = &auth.RequestAuth{
			UseConfigToken: true,
			CallerID:       "keepalive",
			UserAgent:      "ds2api-keepalive",
			AccountID:      id,
			TargetAccount:  id,
			Account:        acc,
			DeepSeekToken:  acc.Token,
			TriedAccounts:  map[string]bool{},
		}
	}
	ctx = auth.WithAuth(ctx, a)

	if m.resolver != nil {
		if err := m.resolver.EnsureManagedToken(ctx, a); err != nil {
			config.Logger.Warn("[keepalive] failed to ensure token", "account", a.AccountID, "error", err)
			return
		}
	}

	startedAt := time.Now()
	var entryID string
	// 仅当用户开启了“记录保活到响应记录”开关时，才写入聊天历史，避免刷屏
	recordHistory := m.store.KeepAliveRecordHistory() && m.chatHistory != nil && m.chatHistory.Enabled()
	if recordHistory {
		entry, err := m.chatHistory.Start(chathistory.StartParams{
			CallerID:    "keepalive",
			UserAgent:   "ds2api-keepalive",
			AccountID:   a.AccountID,
			Surface:     "keepalive",
			Model:       "deepseek-chat",
			Stream:      false,
			UserInput:   promptText,
			Messages:    []chathistory.Message{{Role: "user", Content: promptText}},
			FinalPrompt: promptText,
		})
		if err != nil {
			config.Logger.Warn("[keepalive] failed to start chat history", "account", a.AccountID, "error", err)
		} else {
			entryID = entry.ID
		}
	}

	recordFinish := func(status string, content, reasoning string, statusCode int, errMsg string) {
		if !recordHistory || entryID == "" {
			return
		}
		elapsed := time.Since(startedAt).Milliseconds()
		params := chathistory.UpdateParams{
			Status:           status,
			Content:          content,
			ReasoningContent: reasoning,
			Error:            errMsg,
			StatusCode:       statusCode,
			ElapsedMs:        elapsed,
			FinishReason:     "stop",
			Completed:        true,
		}
		if status == "error" {
			params.FinishReason = "error"
		} else {
			turn := assistantturn.Turn{
				Text:     content,
				Thinking: reasoning,
				Usage:    assistantturn.BuildUsage("deepseek-chat", promptText, reasoning, content, 0),
			}
			params.Usage = assistantturn.OpenAIChatUsage(turn)
		}
		if _, updErr := m.chatHistory.Update(entryID, params); updErr != nil {
			config.Logger.Warn("[keepalive] failed to update chat history", "account", a.AccountID, "error", updErr)
		}
	}

	if m.ds == nil {
		recordFinish("error", "", "", http.StatusInternalServerError, "deepseek client not initialized")
		return
	}

	sessionID, err := m.ds.CreateSession(ctx, a, 2)
	if err != nil {
		config.Logger.Warn("[keepalive] failed to create session", "account", a.AccountID, "error", err)
		recordFinish("error", "", "", http.StatusBadGateway, err.Error())
		return
	}

	// 会话删除策略：仅支持 single 和 none。传入 all 也按照 single 来，避免误删用户在此期间正常生成的会话。
	defer func() {
		mode := m.store.AutoDeleteMode()
		if mode == "none" || a.DeepSeekToken == "" || sessionID == "" {
			return
		}
		deleteCtx, deleteCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer deleteCancel()
		deleteCtx = auth.WithAuth(deleteCtx, a)
		if _, delErr := m.ds.DeleteSessionForToken(deleteCtx, a.DeepSeekToken, sessionID); delErr != nil {
			config.Logger.Warn("[keepalive] failed to auto delete session", "account", a.AccountID, "session_id", sessionID, "error", delErr)
		} else {
			config.Logger.Debug("[keepalive] session deleted successfully", "account", a.AccountID, "session_id", sessionID)
		}
	}()

	powResp, err := m.ds.GetPow(ctx, a, 2)
	if err != nil {
		config.Logger.Warn("[keepalive] failed to get pow", "account", a.AccountID, "error", err)
		recordFinish("error", "", "", http.StatusBadGateway, err.Error())
		return
	}

	// 最正常的说话方式：纯文本，无任何包装或格式注入，开启思考和搜索
	payload := map[string]any{
		"chat_session_id":   sessionID,
		"parent_message_id": nil,
		"model_type":        "default",
		"prompt":            promptText,
		"ref_file_ids":      []any{},
		"thinking_enabled":  true,
		"search_enabled":    true,
		"action":            nil,
		"preempt":           false,
	}

	resp, err := m.ds.CallCompletion(ctx, a, payload, powResp, 2)
	if err != nil {
		config.Logger.Warn("[keepalive] completion call failed", "account", a.AccountID, "error", err)
		recordFinish("error", "", "", http.StatusBadGateway, err.Error())
		return
	}
	if resp.StatusCode != http.StatusOK {
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				config.Logger.Warn("[keepalive] failed to close response body", "account", a.AccountID, "error", cerr)
			}
		}()
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if rerr != nil {
			config.Logger.Debug("[keepalive] reading error response body failed", "account", a.AccountID, "error", rerr)
		}
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		config.Logger.Warn("[keepalive] completion returned non-200 status", "account", a.AccountID, "status", resp.StatusCode, "body", msg)
		recordFinish("error", "", "", resp.StatusCode, msg)
		return
	}

	collected := sse.CollectStreamWithOptions(resp, true, true, sse.CollectOptions{Context: ctx})
	if collected.ReadError != nil {
		config.Logger.Warn("[keepalive] upstream stream interrupted", "account", a.AccountID, "error", collected.ReadError)
		recordFinish("error", collected.Text, collected.Thinking, http.StatusBadGateway, collected.ReadError.Error())
		return
	}
	if collected.UpstreamError != "" {
		config.Logger.Warn("[keepalive] upstream returned error", "account", a.AccountID, "error", collected.UpstreamError)
		recordFinish("error", collected.Text, collected.Thinking, http.StatusBadGateway, collected.UpstreamError)
		return
	}
	if strings.TrimSpace(collected.Text) == "" && strings.TrimSpace(collected.Thinking) == "" {
		config.Logger.Warn("[keepalive] upstream returned empty output", "account", a.AccountID)
		recordFinish("error", "", "", http.StatusBadGateway, "upstream returned empty output")
		return
	}

	config.Logger.Info("[keepalive] keep-alive completed successfully", "account", a.AccountID, "elapsed_ms", time.Since(startedAt).Milliseconds())
	recordFinish("success", collected.Text, collected.Thinking, http.StatusOK, "")
}

// Close gracefully cancels all active keep-alive timers and waits for in-flight requests.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.stopped = true
	for _, entry := range m.timers {
		if entry != nil && entry.timer != nil {
			entry.timer.Stop()
			entry.active = false
		}
	}
	m.timers = make(map[string]*accountTimer)
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()
}
