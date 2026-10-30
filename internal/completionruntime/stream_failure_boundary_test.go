package completionruntime

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestStreamSwitchStartupFailureWritesOnlyOneError(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["managed"],"accounts":[{"email":"a@test.com","token":"ta"},{"email":"b@test.com","token":"tb"}]}`)
	store := config.LoadStore()
	r := auth.NewResolver(store, account.NewPool(store), func(_ context.Context, acc config.Account) (string, error) { return "token-" + acc.Identifier(), nil })
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer managed")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release(a)
	ds := &fakeDeepSeekCaller{sessionErrors: []error{errors.New("unavailable")}}
	errorsSeen, finalizations := 0, 0
	ExecuteStreamWithRetry(context.Background(), ds, a, sseHTTPResponse(503, "unavailable"), map[string]any{"prompt": "original"}, "pow", StreamRetryOptions{RetryEnabled: true}, StreamRetryHooks{
		ConsumeAttempt: func(*http.Response, bool) ConsumeAttemptResult {
			t.Fatal("non-SSE error was passed to consumer")
			return ConsumeAttemptResult{}
		},
		OnRetryFailure: func(int, string, string) { errorsSeen++ },
		Finalize:       func(int) { finalizations++ },
	})
	if errorsSeen != 1 || finalizations != 0 {
		t.Fatalf("terminal error duplicated: errors=%d finalizations=%d", errorsSeen, finalizations)
	}
}

func TestStreamToolRetryExhaustionUsesStoredFailure(t *testing.T) {
	ds := &fakeDeepSeekCaller{}
	initial := sseHTTPResponse(200, "")
	errorsSeen, finalizations, resets := 0, 0, 0
	ExecuteStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, initial, map[string]any{"prompt": "original"}, "pow", StreamRetryOptions{RetryEnabled: true}, StreamRetryHooks{
		ConsumeAttempt: func(resp *http.Response, _ bool) ConsumeAttemptResult {
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			return ConsumeAttemptResult{Retryable: true}
		},
		EmptyOutputError: func() *assistantturn.OutputError {
			return &assistantturn.OutputError{Status: 422, Message: "bad tool", Code: "invalid_tool_arguments"}
		},
		OnRegenerate: func() { resets++ },
		OnRetryFailure: func(_ int, _ string, code string) {
			errorsSeen++
			if code != "invalid_tool_arguments" {
				t.Fatalf("lost tool failure: %s", code)
			}
		},
		Finalize: func(int) { finalizations++ },
	})
	if errorsSeen != 1 || finalizations != 0 || resets != 1 || len(ds.payloads) != 1 {
		t.Fatalf("unbounded or repeated finalization: errors=%d finalizations=%d resets=%d calls=%d", errorsSeen, finalizations, resets, len(ds.payloads))
	}
}
