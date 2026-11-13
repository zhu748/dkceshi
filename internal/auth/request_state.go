package auth

import (
	"time"

	"ds2api/internal/config"
)

type RequestAuth struct {
	// ManagedCredential identifies a configured key even before acquiring a slot.
	ManagedCredential bool
	UseConfigToken    bool
	DeepSeekToken     string
	CallerID          string
	UserAgent         string
	AccountID         string
	TargetAccount     string
	// FileAccountID binds references whose source bytes cannot be replayed.
	FileAccountID        string
	accountPins          int
	accountSwitches      int
	accountSwitchFailure string
	failoverWait         time.Duration
	accountCleanup       func(bool)
	Account              config.Account
	TriedAccounts        map[string]bool
	// ToolsPresent 表示本次请求是否携带工具定义（请求 body 的 tools 非空）。
	// 它决定账号池调度：含工具请求走默认/仅工具池，无工具请求走无工具池。
	// 账号切换重试时沿用该标志，保证切换后仍遵守号池约束。
	ToolsPresent bool
	// ToolMarker 是本次调用者专属的工具调用标识（由服务端密钥派生，按 Key 稳定）。
	// 提示词渲染输出该标识，模型输出在进入解析器前会被归一化回 EPSE。
	ToolMarker string
	// authRefreshState 记录本次请求内每个账号的强制重新登录结果（成功/失败），
	// 避免同一账号被反复刷新 Token 造成死循环；换号后新账号拥有独立的刷新额度。
	authRefreshState map[string]string
	// authUsedAccounts 记录本次请求内成功调度（拿到可用 Token）过的账号；
	// authFailedAccounts 记录本次请求内发生过鉴权失败的账号。二者供请求结束时
	// 结算连续鉴权失败计数（见 Resolver.settleAuthFailures）。
	authUsedAccounts   map[string]bool
	authFailedAccounts map[string]bool
	// releasedAccounts 记录本次请求已经把名额还给号池的账号。SwitchAccount 在换号
	// 前先归还当前账号的名额（号池打满时才换得动），失败后请求仍带着这个已归还的
	// AccountID 走到收尾的 Release；没有这份记录，收尾会对同一账号再减一次引用计数，
	// 使号池把该账号的并发持有数算少，放行超过 account_max_inflight 的请求。
	releasedAccounts map[string]bool
	resolver         *Resolver
}
