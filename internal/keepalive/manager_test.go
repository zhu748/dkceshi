package keepalive

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

func newTestStore(t *testing.T, cfgJSON string) *config.Store {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", cfgJSON)
	store, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	return store
}

func TestParseIntervalRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "range with hyphen",
			input:   "2-4",
			wantMin: 2 * time.Hour,
			wantMax: 4 * time.Hour,
		},
		{
			name:    "range with spaces and hours suffix",
			input:   " 2.5 - 5h ",
			wantMin: 150 * time.Minute,
			wantMax: 5 * time.Hour,
		},
		{
			name:    "inverted range",
			input:   "4-2",
			wantMin: 2 * time.Hour,
			wantMax: 4 * time.Hour,
		},
		{
			name:    "single number",
			input:   "3",
			wantMin: 3 * time.Hour,
			wantMax: 3 * time.Hour,
		},
		{
			name:    "single float",
			input:   "1.5",
			wantMin: 90 * time.Minute,
			wantMax: 90 * time.Minute,
		},
		{
			name:    "invalid text fallback",
			input:   "invalid",
			wantMin: 2 * time.Hour,
			wantMax: 4 * time.Hour,
		},
		{
			name:    "empty fallback",
			input:   "",
			wantMin: 2 * time.Hour,
			wantMax: 4 * time.Hour,
		},
		{
			name:    "negative fallback",
			input:   "-1",
			wantMin: 2 * time.Hour,
			wantMax: 4 * time.Hour,
		},
		{
			name:    "extreme large value clamped to 720h",
			input:   "1e19",
			wantMin: 720 * time.Hour,
			wantMax: 720 * time.Hour,
		},
		{
			name:    "extremely small value clamped to 0.05h (3m)",
			input:   "0.00001",
			wantMin: 3 * time.Minute,
			wantMax: 3 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin, gotMax := ParseIntervalRange(tt.input)
			if gotMin != tt.wantMin {
				t.Errorf("ParseIntervalRange(%q) gotMin = %v, want %v", tt.input, gotMin, tt.wantMin)
			}
			if gotMax != tt.wantMax {
				t.Errorf("ParseIntervalRange(%q) gotMax = %v, want %v", tt.input, gotMax, tt.wantMax)
			}
		})
	}
}

func TestRandomDurationFromInterval(t *testing.T) {
	for i := 0; i < 50; i++ {
		d := RandomDurationFromInterval("2-4")
		if d < 2*time.Hour || d > 4*time.Hour {
			t.Fatalf("RandomDurationFromInterval('2-4') produced out of range duration: %v", d)
		}
	}

	single := RandomDurationFromInterval("3")
	if single != 3*time.Hour {
		t.Fatalf("RandomDurationFromInterval('3') = %v, want 3h", single)
	}
}

func TestSplitAndPickPrompts(t *testing.T) {
	raw := "现在纽约天气如何？;现在几点了？；你好啊;简单介绍一下你自己；9.8-9.11=？"
	list := SplitPrompts(raw)
	if len(list) != 5 {
		t.Fatalf("expected 5 prompts, got %d: %v", len(list), list)
	}

	picked := PickPrompt(raw)
	found := false
	for _, p := range list {
		if p == picked {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("picked prompt %q not found in list %v", picked, list)
	}

	// Empty fallback
	emptyPicked := PickPrompt("  ;  ； ")
	if strings.TrimSpace(emptyPicked) == "" {
		t.Fatalf("expected non-empty prompt from fallback, got empty")
	}
}

func TestAccountTimerMonostableLifecycle(t *testing.T) {
	store := newTestStore(t, `{
		"accounts": [{"email": "test1@example.com"}],
		"keep_alive": {"enabled": true, "interval": "2-4"}
	}`)

	var scheduledCalls int32
	var timerTriggered int32

	mgr := NewManager(store, nil, nil, nil, nil)
	var triggerFunc func()
	var mu sync.Mutex
	mgr.afterFunc = func(d time.Duration, f func()) *time.Timer {
		atomic.AddInt32(&scheduledCalls, 1)
		mu.Lock()
		triggerFunc = f
		mu.Unlock()
		return time.NewTimer(time.Hour)
	}

	accID := "test1@example.com"

	// 1. First request starts the timer
	mgr.OnAccountRequest(accID)
	if atomic.LoadInt32(&scheduledCalls) != 1 {
		t.Fatalf("expected 1 scheduled timer call, got %d", scheduledCalls)
	}

	// 2. Second request during active timer is ignored ("期间再有请求不管")
	mgr.OnAccountRequest(accID)
	mgr.OnAccountRequest(accID)
	if atomic.LoadInt32(&scheduledCalls) != 1 {
		t.Fatalf("expected still 1 scheduled timer call, got %d", scheduledCalls)
	}

	// 3. Timer expires -> triggers keep-alive -> resets timer
	mu.Lock()
	tf := triggerFunc
	mu.Unlock()
	if tf != nil {
		atomic.AddInt32(&timerTriggered, 1)
		tf()
	}

	// Check that timer is reset and active is false
	mgr.mu.Lock()
	entry, ok := mgr.timers[accID]
	if !ok || entry.active {
		t.Fatalf("expected entry to exist and be inactive, got ok=%v, active=%v", ok, entry.active)
	}
	mgr.mu.Unlock()

	// 4. Next request after reset can start a new timer
	mgr.OnAccountRequest(accID)
	if atomic.LoadInt32(&scheduledCalls) != 2 {
		t.Fatalf("expected 2 scheduled timer calls after reset, got %d", scheduledCalls)
	}

	// 5. Close stops all timers
	mgr.Close()
	mgr.mu.Lock()
	if !mgr.stopped {
		t.Fatalf("expected manager to be stopped")
	}
	mgr.mu.Unlock()
}

func TestKeepAliveDisabledDoesNothing(t *testing.T) {
	store := newTestStore(t, `{
		"accounts": [{"email": "test1@example.com"}],
		"keep_alive": {"enabled": false}
	}`)

	var scheduledCalls int32
	mgr := NewManager(store, nil, nil, nil, nil)
	mgr.afterFunc = func(d time.Duration, f func()) *time.Timer {
		atomic.AddInt32(&scheduledCalls, 1)
		return time.NewTimer(time.Hour)
	}

	mgr.OnAccountRequest("test1@example.com")
	if atomic.LoadInt32(&scheduledCalls) != 0 {
		t.Fatalf("expected 0 timer calls when disabled, got %d", scheduledCalls)
	}
}

func TestTriggerKeepAliveBusyRetry(t *testing.T) {
	store := newTestStore(t, `{
		"accounts": [{"email": "busy@example.com"}],
		"keep_alive": {"enabled": true, "interval": "2-4"},
		"runtime": {"account_max_inflight": 1}
	}`)
	pool := account.NewPool(store)
	// 占用该账号并发
	_, ok := pool.AcquireKeepAlive("busy@example.com")
	if !ok {
		t.Fatal("failed to acquire initial slot")
	}

	var scheduledRetries int32
	var mu sync.Mutex
	var nextTrigger func()

	mgr := NewManager(store, pool, nil, nil, nil)
	mgr.afterFunc = func(d time.Duration, f func()) *time.Timer {
		atomic.AddInt32(&scheduledRetries, 1)
		mu.Lock()
		nextTrigger = f
		mu.Unlock()
		return time.NewTimer(time.Hour)
	}

	// 标记 active
	mgr.mu.Lock()
	mgr.timers["busy@example.com"] = &accountTimer{active: true}
	mgr.mu.Unlock()

	// 触发保活，因为账号正忙，应安排重试
	mgr.triggerKeepAlive("busy@example.com")

	if atomic.LoadInt32(&scheduledRetries) != 1 {
		t.Fatalf("expected 1 scheduled retry call, got %d", scheduledRetries)
	}

	mgr.mu.Lock()
	entry := mgr.timers["busy@example.com"]
	if !entry.active || entry.retryCount != 1 {
		t.Fatalf("expected entry to remain active with retryCount=1, got active=%v, retryCount=%d", entry.active, entry.retryCount)
	}
	mgr.mu.Unlock()

	// 释放账号，使后续重试成功
	pool.Release("busy@example.com")

	mu.Lock()
	retryFn := nextTrigger
	mu.Unlock()

	if retryFn != nil {
		retryFn()
	}

	// 此时应成功获取槽位并执行完毕，active 重置为 false
	mgr.mu.Lock()
	entry = mgr.timers["busy@example.com"]
	if entry.active {
		t.Fatal("expected entry active to be reset to false after successful acquire")
	}
	mgr.mu.Unlock()
}

func TestTriggerKeepAlivePanicRecovery(t *testing.T) {
	store := newTestStore(t, `{
		"accounts": [{"email": "panic@example.com"}],
		"keep_alive": {"enabled": true, "interval": "2-4"}
	}`)
	mgr := NewManager(store, nil, nil, nil, nil)
	// 强制注入会触发异常的场景，验证 triggerKeepAlive 的 recover 保护
	mgr.mu.Lock()
	mgr.timers["panic@example.com"] = &accountTimer{active: true}
	mgr.mu.Unlock()

	// 不应该 panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("triggerKeepAlive should not escape panic, got %v", r)
		}
	}()
	mgr.triggerKeepAlive("panic@example.com")
}

func TestRecordHistorySwitchControlsLogging(t *testing.T) {
	chatHistoryPath := t.TempDir() + "/chat_history.json"
	chatStore := chathistory.New(chatHistoryPath)

	// 1. 当 record_history 为 false 时，不记录历史
	storeOff := newTestStore(t, `{
		"accounts": [{"email": "rec@example.com", "token": "tok"}],
		"keep_alive": {"enabled": true, "record_history": false}
	}`)
	if storeOff.KeepAliveRecordHistory() {
		t.Fatal("expected KeepAliveRecordHistory to be false")
	}

	mgrOff := NewManager(storeOff, nil, nil, nil, chatStore)
	mgrOff.executeKeepAlive(context.Background(), config.Account{Email: "rec@example.com", Token: "tok"})

	snapOff, _ := chatStore.Snapshot()
	if len(snapOff.Items) != 0 {
		t.Fatalf("expected 0 chat history items when record_history is false, got %d", len(snapOff.Items))
	}

	// 2. 当 record_history 为 true 时，记录历史
	storeOn := newTestStore(t, `{
		"accounts": [{"email": "rec@example.com", "token": "tok"}],
		"keep_alive": {"enabled": true, "record_history": true}
	}`)
	if !storeOn.KeepAliveRecordHistory() {
		t.Fatal("expected KeepAliveRecordHistory to be true")
	}
	mgrOn := NewManager(storeOn, nil, nil, nil, chatStore)
	// 因为 ds == nil，会在 chatHistory 里记录一条 error
	mgrOn.executeKeepAlive(context.Background(), config.Account{Email: "rec@example.com", Token: "tok"})

	snapOn, _ := chatStore.Snapshot()
	if len(snapOn.Items) != 1 {
		t.Fatalf("expected 1 chat history item when record_history is true, got %d", len(snapOn.Items))
	}
	if snapOn.Items[0].Surface != "keepalive" {
		t.Fatalf("expected surface keepalive, got %s", snapOn.Items[0].Surface)
	}
}

type fakeKeepAliveDS struct {
	singleDeleteCalls int32
	allDeleteCalls    int32
	capturedAuth      *auth.RequestAuth
	capturedContext   context.Context
	completionResp    *http.Response
}

func (f *fakeKeepAliveDS) CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error) {
	f.capturedAuth = a
	f.capturedContext = ctx
	return "test-session-123", nil
}

func (f *fakeKeepAliveDS) GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error) {
	return "pow-test", nil
}

func (f *fakeKeepAliveDS) CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error) {
	if f.completionResp != nil {
		return f.completionResp, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: {\"v\":{\"response\":{\"fragments\":[{\"content\":\"Hello\",\"type\":\"RESPONSE\"}]}}}\n\ndata: [DONE]\n\n")),
	}, nil
}

func (f *fakeKeepAliveDS) DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	atomic.AddInt32(&f.singleDeleteCalls, 1)
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func TestKeepAliveSessionDeletePolicySafety(t *testing.T) {
	// 无论系统配置是 single 还是 all，保活请求都只删除单会话，绝不调用 delete_all
	fakeDS := &fakeKeepAliveDS{}

	storeAll := newTestStore(t, `{
		"accounts": [{"email": "safe@example.com", "token": "tok"}],
		"keep_alive": {"enabled": true},
		"auto_delete": {"mode": "all"}
	}`)

	pool := account.NewPool(storeAll)
	resolver := auth.NewResolver(storeAll, pool, nil)
	mgr := NewManager(storeAll, pool, fakeDS, resolver, nil)

	// executeKeepAlive 启动并退出后，检查是否只调了单会话删除
	mgr.executeKeepAlive(context.Background(), config.Account{Email: "safe@example.com", Token: "tok"})

	if atomic.LoadInt32(&fakeDS.allDeleteCalls) != 0 {
		t.Fatalf("keepalive must NEVER call delete_all even when mode is all, got %d calls", fakeDS.allDeleteCalls)
	}
	if atomic.LoadInt32(&fakeDS.singleDeleteCalls) != 1 {
		t.Fatalf("expected exactly 1 single session delete call, got %d", fakeDS.singleDeleteCalls)
	}

	// 验证 TargetAccount 被牢牢锁定，防止 SwitchAccount
	if fakeDS.capturedAuth == nil || fakeDS.capturedAuth.TargetAccount != "safe@example.com" {
		t.Fatalf("expected TargetAccount to be locked to safe@example.com, got %v", fakeDS.capturedAuth)
	}

	// 验证 context 中注入了 auth.FromContext
	if authFromCtx, ok := auth.FromContext(fakeDS.capturedContext); !ok || authFromCtx == nil || authFromCtx.AccountID != "safe@example.com" {
		t.Fatalf("expected auth.FromContext in captured context, got %v", authFromCtx)
	}
}

func TestKeepAliveCallCompletionNon200Error(t *testing.T) {
	chatHistoryPath := t.TempDir() + "/chat_history.json"
	chatStore := chathistory.New(chatHistoryPath)

	fakeDS := &fakeKeepAliveDS{
		completionResp: &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("upstream 502 bad gateway")),
		},
	}

	store := newTestStore(t, `{
		"accounts": [{"email": "err@example.com", "token": "tok"}],
		"keep_alive": {"enabled": true, "record_history": true}
	}`)

	mgr := NewManager(store, nil, fakeDS, nil, chatStore)
	mgr.executeKeepAlive(context.Background(), config.Account{Email: "err@example.com", Token: "tok"})

	snap, _ := chatStore.Snapshot()
	if len(snap.Items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(snap.Items))
	}
	detail, err := chatStore.Get(snap.Items[0].ID)
	if err != nil {
		t.Fatalf("get history item: %v", err)
	}
	if detail.Status != "error" {
		t.Fatalf("expected history status error, got %s", detail.Status)
	}
	if detail.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status code 502, got %d", detail.StatusCode)
	}
}

func TestKeepAliveForgetsDeletedAccount(t *testing.T) {
	store := newTestStore(t, `{
		"accounts": [{"email": "del@example.com"}],
		"keep_alive": {"enabled": true}
	}`)

	mgr := NewManager(store, nil, nil, nil, nil)
	mgr.OnAccountRequest("del@example.com")

	mgr.mu.Lock()
	if _, ok := mgr.timers["del@example.com"]; !ok {
		mgr.mu.Unlock()
		t.Fatal("expected timer entry to exist")
	}
	mgr.mu.Unlock()

	// 从配置中删除账号
	_ = store.Update(func(c *config.Config) error {
		c.Accounts = nil
		return nil
	})

	// 触发保活，应当检测到账号已删除并调用 ForgetAccount
	mgr.triggerKeepAlive("del@example.com")

	mgr.mu.Lock()
	if _, ok := mgr.timers["del@example.com"]; ok {
		mgr.mu.Unlock()
		t.Fatal("expected timer entry to be removed from map after account deletion")
	}
	mgr.mu.Unlock()
}
