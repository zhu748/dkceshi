package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type autoDeleteModeDSStub struct {
	resp          *http.Response
	singleCalls   int
	allCalls      int
	lastSessionID string
	lastCtxErr    error
}

func (m *autoDeleteModeDSStub) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *autoDeleteModeDSStub) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow", nil
}

func (m *autoDeleteModeDSStub) UploadFile(_ context.Context, _ *auth.RequestAuth, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m *autoDeleteModeDSStub) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ any, _ string, _ int) (*http.Response, error) {
	return m.resp, nil
}

func (m *autoDeleteModeDSStub) DeleteSessionForToken(_ context.Context, _ string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	m.singleCalls++
	m.lastSessionID = sessionID
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func (m *autoDeleteModeDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	m.allCalls++
	return nil
}

func (m *autoDeleteModeDSStub) DeleteSessionForTokenCtx(ctx context.Context, _ string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	m.singleCalls++
	m.lastSessionID = sessionID
	m.lastCtxErr = ctx.Err()
	return &dsclient.DeleteSessionResult{SessionID: sessionID, Success: true}, nil
}

func TestChatCompletionsAutoDeleteModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantSingle int
		wantAll    int
	}{
		{name: "none", mode: "none"},
		{name: "single", mode: "single", wantSingle: 1},
		{name: "all", mode: "all", wantAll: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &autoDeleteModeDSStub{
				resp: makeOpenAISSEHTTPResponse(
					`data: {"p":"response/content","v":"hello"}`,
					"data: [DONE]",
				),
			}
			h := &Handler{
				Store: mockOpenAIConfig{
					autoDeleteMode: tc.mode,
				},
				Auth: streamStatusAuthStub{},
				DS:   ds,
			}

			reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"stream":false}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
			req.Header.Set("Authorization", "Bearer direct-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h.ChatCompletions(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if ds.singleCalls != tc.wantSingle {
				t.Fatalf("single delete calls=%d want=%d", ds.singleCalls, tc.wantSingle)
			}
			if ds.allCalls != tc.wantAll {
				t.Fatalf("all delete calls=%d want=%d", ds.allCalls, tc.wantAll)
			}
			if tc.wantSingle > 0 && ds.lastSessionID != "session-id" {
				t.Fatalf("expected single delete for session-id, got %q", ds.lastSessionID)
			}
		})
	}
}

type autoDeleteCtxDSStub struct {
	autoDeleteModeDSStub
}

func (m *autoDeleteCtxDSStub) DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*dsclient.DeleteSessionResult, error) {
	return m.DeleteSessionForTokenCtx(ctx, token, sessionID)
}

func (m *autoDeleteCtxDSStub) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	m.allCalls++
	return nil
}

func TestAutoDeleteRemoteSessionIgnoresCanceledParentContext(t *testing.T) {
	ds := &autoDeleteCtxDSStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			autoDeleteMode: "single",
		},
		DS: ds,
	}
	a := &auth.RequestAuth{DeepSeekToken: "token", AccountID: "acct"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h.autoDeleteRemoteSession(ctx, a, "session-id", "deepseek-v4-flash")

	if ds.singleCalls != 1 {
		t.Fatalf("single delete calls=%d want=1", ds.singleCalls)
	}
	if ds.lastCtxErr != nil {
		t.Fatalf("delete ctx should not inherit cancellation, got %v", ds.lastCtxErr)
	}
}

// TestAutoDeleteRemoteSessionSuffixModelOverridesNone 验证 -autodelete 后缀模型
// 在全局 mode=none 时降级为删除本次对话，对应 App 抓包里的
// POST /api/v0/chat_session/delete 动作。
func TestAutoDeleteRemoteSessionSuffixModelOverridesNone(t *testing.T) {
	tests := []struct {
		name          string
		globalMode    string
		resolvedModel string
		wantSingle    int
		wantAll       int
	}{
		{
			name:          "none + suffix model -> single delete",
			globalMode:    "none",
			resolvedModel: "deepseek-v4-flash-autodelete",
			wantSingle:    1,
		},
		{
			name:          "single + suffix model -> still single (no double delete)",
			globalMode:    "single",
			resolvedModel: "deepseek-v4-flash-autodelete",
			wantSingle:    1,
		},
		{
			name:          "all + suffix model -> all (global wins)",
			globalMode:    "all",
			resolvedModel: "deepseek-v4-flash-autodelete",
			wantAll:       1,
		},
		{
			name:          "none + plain model -> no delete",
			globalMode:    "none",
			resolvedModel: "deepseek-v4-flash",
		},
		{
			name:          "none + alias-resolved suffix model -> single delete",
			globalMode:    "none",
			resolvedModel: "deepseek-v4-pro-search-autodelete",
			wantSingle:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ds := &autoDeleteModeDSStub{}
			h := &Handler{
				Store: mockOpenAIConfig{
					autoDeleteMode: tc.globalMode,
				},
				DS: ds,
			}
			a := &auth.RequestAuth{DeepSeekToken: "token", AccountID: "acct"}

			h.autoDeleteRemoteSession(context.Background(), a, "session-id", tc.resolvedModel)

			if ds.singleCalls != tc.wantSingle {
				t.Fatalf("single delete calls=%d want=%d", ds.singleCalls, tc.wantSingle)
			}
			if ds.allCalls != tc.wantAll {
				t.Fatalf("all delete calls=%d want=%d", ds.allCalls, tc.wantAll)
			}
		})
	}
}
