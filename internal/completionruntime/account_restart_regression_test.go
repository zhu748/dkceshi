package completionruntime

import (
	"context"
	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func restartAuthFixture(t *testing.T) (*auth.Resolver, *auth.RequestAuth) {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"},{"email":"c@example.com","token":"tc"}]}`)
	s := config.LoadStore()
	p := account.NewPool(s)
	t.Cleanup(p.Close)
	r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer key")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Release(a) })
	return r, a
}

func TestFailoverPreparationContinuesToThirdAccount(t *testing.T) {
	_, a := restartAuthFixture(t)
	ds := &fakeDeepSeekCaller{sessionErrors: []error{errors.New("replacement account session unavailable")}}
	req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "hello"}
	start := StartResult{Request: req, Payload: req.CompletionPayload("initial"), Response: sseHTTPResponse(200, "data: [DONE]"), Pow: "pow", SessionID: "initial"}
	_, err := ExecuteNonStreamStartedWithRetry(context.Background(), ds, a, start, Options{RetryEnabled: true})
	if err != nil {
		t.Fatalf("healthy third account unused: owner=%s error=%+v completions=%v", a.AccountID, err, ds.completionAccounts)
	}
}

func TestStreamFailoverPreparationContinuesToThirdAccount(t *testing.T) {
	_, a := restartAuthFixture(t)
	ds := &fakeDeepSeekCaller{sessionErrors: []error{errors.New("replacement account session unavailable")}}
	req := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "hello"}
	calls := 0
	failure := ""
	ExecuteStreamWithRetry(context.Background(), ds, a, sseHTTPResponse(200, "data: [DONE]"), req.CompletionPayload("initial"), "pow", StreamRetryOptions{RetryEnabled: true, Request: req}, StreamRetryHooks{
		ConsumeAttempt: func(_ *http.Response, _ bool) ConsumeAttemptResult {
			calls++
			if calls == 1 {
				return ConsumeAttemptResult{Retryable: true}
			}
			return ConsumeAttemptResult{Terminal: true}
		},
		EmptyOutputError: func() *assistantturn.OutputError {
			return &assistantturn.OutputError{Status: 503, Code: "upstream_unavailable", Message: assistantturn.UpstreamUnavailableEmptyMessage}
		},
		OnRetryFailure: func(_ int, message, _ string) { failure = message },
	})
	if calls < 2 {
		t.Fatalf("healthy third account unused: owner=%s failure=%s consumed=%d", a.AccountID, failure, calls)
	}
}

func TestRequestReleaseCleansFinalSession(t *testing.T) {
	r, a := restartAuthFixture(t)
	ds := &ownedCleanupDS{events: make(chan ownedDeletion, 1)}
	_, err := createCompletionSession(context.Background(), ds, a, 1, cleanupPolicy{mode: "single"})
	if err != nil {
		t.Fatal(err)
	}
	r.Release(a)
	select {
	case <-ds.events:
	case <-time.After(time.Second):
		t.Fatal("request ended with registered session cleanup never invoked")
	}
}
