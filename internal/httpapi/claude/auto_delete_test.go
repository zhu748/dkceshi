package claude

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

// claudeAutoDeleteDSStub 模拟 DeepSeek 上游，统计 delete 调用次数。
type claudeAutoDeleteDSStub struct {
	mu                sync.Mutex
	createSessionErr  error
	completionResp    *http.Response
	singleDeleteCalls int
	allDeleteCalls    int
	lastSessionID     string
}

func (s *claudeAutoDeleteDSStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	if s.createSessionErr != nil {
		return "", s.createSessionErr
	}
	return "claude-session-id", nil
}

func (s *claudeAutoDeleteDSStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (s *claudeAutoDeleteDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id"}, nil
}

func (s *claudeAutoDeleteDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ any, _ string, _ int) (*http.Response, error) {
	if s.completionResp == nil {
		// 默认返回一个最小可用的 SSE 响应，让 non-stream 路径能完成
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("data: {\"p\":\"response/content\",\"v\":\"ok\"}\ndata: [DONE]\n")),
		}, nil
	}
	return s.completionResp, nil
}

func (s *claudeAutoDeleteDSStub) DeleteSessionForToken(_ context.Context, _ string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.singleDeleteCalls++
	s.lastSessionID = sessionID
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (s *claudeAutoDeleteDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allDeleteCalls++
	return nil
}

func (s *claudeAutoDeleteDSStub) stats() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.singleDeleteCalls, s.allDeleteCalls
}

// claudeAutoDeleteAuthStub 暴露固定 token，让 autoDeleteRemoteSession 能进入实际删除分支。
type claudeAutoDeleteAuthStub struct{}

func (claudeAutoDeleteAuthStub) Determine(*http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{
		DeepSeekToken: "direct-token",
		CallerID:      "caller:test",
		TriedAccounts: map[string]bool{},
	}, nil
}

func (claudeAutoDeleteAuthStub) Release(*auth.RequestAuth) {}

// TestClaudeAutoDeleteSuffixModelOverridesNone 验证 -autodelete 后缀模型在 Claude 入口
// 按以下策略联动全局 auto_delete.mode（与 OpenAI chat handler 完全对齐）：
//   - none + -autodelete 模型   → 删除本次对话
//   - single + -autodelete 模型 → 仍然 single（不重复删除）
//   - all + -autodelete 模型    → all（全局策略优先）
//   - none + 普通模型           → 不删除
//   - single + 普通模型         → 删除本次对话
//   - all + 普通模型            → 删除全部
//
// 对应 App 抓包里的 POST /api/v0/chat_session/delete 动作。
func TestClaudeAutoDeleteSuffixModelOverridesNone(t *testing.T) {
	tests := []struct {
		name       string
		globalMode string
		model      string
		wantSingle int
		wantAll    int
	}{
		{
			name:       "none + plain claude model -> no delete",
			globalMode: "none",
			model:      "claude-sonnet-4-6",
		},
		{
			name:       "none + claude-sonnet-4-6-autodelete -> single delete",
			globalMode: "none",
			model:      "claude-sonnet-4-6-autodelete",
			wantSingle: 1,
		},
		{
			name:       "none + claude-opus-4-6-autodelete -> single delete (alias-resolved)",
			globalMode: "none",
			model:      "claude-opus-4-6-autodelete",
			wantSingle: 1,
		},
		{
			// 用户用 Anthropic SDK 直接传 DeepSeek 后缀模型也应当生效
			name:       "none + deepseek-v4-flash-autodelete (direct) -> single delete",
			globalMode: "none",
			model:      "deepseek-v4-flash-autodelete",
			wantSingle: 1,
		},
		{
			name:       "none + deepseek-v4-pro-autodelete (direct) -> single delete",
			globalMode: "none",
			model:      "deepseek-v4-pro-autodelete",
			wantSingle: 1,
		},
		{
			name:       "none + deepseek-v4-pro-search-autodelete (direct) -> single delete",
			globalMode: "none",
			model:      "deepseek-v4-pro-search-autodelete",
			wantSingle: 1,
		},
		{
			// 普通 deepseek 模型直传，全局 none 时不删除（与 claude-* base 行为一致）
			name:       "none + deepseek-v4-flash (direct, plain) -> no delete",
			globalMode: "none",
			model:      "deepseek-v4-flash",
		},
		{
			name:       "single + plain model -> single delete",
			globalMode: "single",
			model:      "claude-sonnet-4-6",
			wantSingle: 1,
		},
		{
			name:       "single + -autodelete model -> single delete (no double)",
			globalMode: "single",
			model:      "claude-sonnet-4-6-autodelete",
			wantSingle: 1,
		},
		{
			name:       "all + plain model -> all delete",
			globalMode: "all",
			model:      "claude-sonnet-4-6",
			wantAll:    1,
		},
		{
			name:       "all + -autodelete model -> all delete (global wins)",
			globalMode: "all",
			model:      "claude-sonnet-4-6-autodelete",
			wantAll:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &claudeAutoDeleteDSStub{}
			h := &Handler{
				Store: mockClaudeConfig{
					autoDeleteMode: tc.globalMode,
				},
				Auth: claudeAutoDeleteAuthStub{},
				DS:   ds,
			}
			reqBody := `{"model":"` + tc.model + `","messages":[{"role":"user","content":"hi"}],"max_tokens":1024,"stream":false}`
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.Messages(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			singleCalls, allCalls := ds.stats()
			if singleCalls != tc.wantSingle {
				t.Fatalf("single delete calls=%d want=%d", singleCalls, tc.wantSingle)
			}
			if allCalls != tc.wantAll {
				t.Fatalf("all delete calls=%d want=%d", allCalls, tc.wantAll)
			}
			if tc.wantSingle > 0 && ds.lastSessionID != "claude-session-id" {
				t.Fatalf("expected single delete for claude-session-id, got %q", ds.lastSessionID)
			}
		})
	}
}

// TestClaudeAutoDeleteRemoteSessionDirectly 单独覆盖 autoDeleteRemoteSession 方法
// 的策略计算，避免依赖完整 HTTP 路径。
func TestClaudeAutoDeleteRemoteSessionDirectly(t *testing.T) {
	tests := []struct {
		name          string
		globalMode    string
		resolvedModel string
		wantSingle    int
		wantAll       int
	}{
		{name: "none + plain -> skip", globalMode: "none", resolvedModel: "deepseek-v4-flash"},
		{name: "none + suffix -> single", globalMode: "none", resolvedModel: "deepseek-v4-flash-autodelete", wantSingle: 1},
		{name: "single + suffix -> single", globalMode: "single", resolvedModel: "deepseek-v4-flash-autodelete", wantSingle: 1},
		{name: "all + suffix -> all", globalMode: "all", resolvedModel: "deepseek-v4-flash-autodelete", wantAll: 1},
		{name: "single + plain -> single", globalMode: "single", resolvedModel: "deepseek-v4-flash", wantSingle: 1},
		{name: "all + plain -> all", globalMode: "all", resolvedModel: "deepseek-v4-flash", wantAll: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &claudeAutoDeleteDSStub{}
			h := &Handler{
				Store: mockClaudeConfig{autoDeleteMode: tc.globalMode},
				DS:    ds,
			}
			a := &auth.RequestAuth{DeepSeekToken: "token", AccountID: "acct"}

			h.autoDeleteRemoteSession(context.Background(), a, "session-id", tc.resolvedModel)

			singleCalls, allCalls := ds.stats()
			if singleCalls != tc.wantSingle {
				t.Fatalf("single delete calls=%d want=%d", singleCalls, tc.wantSingle)
			}
			if allCalls != tc.wantAll {
				t.Fatalf("all delete calls=%d want=%d", allCalls, tc.wantAll)
			}
		})
	}
}
