package completionruntime

import (
	"context"
	"net/http"
	"testing"
	"time"

	"ds2api/internal/assistantturn"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func TestTerminalEmptyStreamRecordsCooldownWithRetriesDisabled(t *testing.T) {
	r, a := restartAuthFixture(t)
	old := a.AccountID
	ds := &fakeDeepSeekCaller{}
	before := time.Now()
	ExecuteStreamWithRetry(context.Background(), ds, a, sseHTTPResponse(200, "data: [DONE]"), map[string]any{}, "pow", StreamRetryOptions{RetryEnabled: false}, StreamRetryHooks{
		ConsumeAttempt: func(resp *http.Response, deferEmpty bool) ConsumeAttemptResult {
			closeRetryBody("test", resp.Body)
			if deferEmpty {
				t.Fatal("disabled retries were enabled")
			}
			return ConsumeAttemptResult{Terminal: true, Failure: &assistantturn.OutputError{Status: 503, Code: "upstream_unavailable", Message: assistantturn.UpstreamUnavailableEmptyMessage}}
		},
	})
	acc, _ := r.Store.FindAccount(old)
	if !acc.IsCoolingDown() || acc.CooldownUntil < float64(before.Add(59*time.Second).Unix()) || a.AccountID != old || len(ds.payloads) != 0 {
		t.Fatalf("terminal failure did not record independent cooldown: %+v", acc)
	}
}

func TestReasoningOnly429ImmediatelyCoolsAndSwitchesAccount(t *testing.T) {
	r, a := restartAuthFixture(t)
	old := a.AccountID
	ds := &fakeDeepSeekCaller{sessionByAccount: true, responses: []*http.Response{
		sseHTTPResponse(200, `data: {"p":"response/thinking_content","v":"thinking without an answer"}`),
		sseHTTPResponse(200, `data: {"p":"response/thinking_content","v":"also limited"}`),
		sseHTTPResponse(200, `data: {"p":"response/content","v":"healthy answer"}`),
	}}
	_, failure := ExecuteNonStreamWithRetry(context.Background(), ds, a, promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "task", Thinking: true}, Options{RetryEnabled: true})
	if failure != nil || len(ds.completionAccounts) != 3 || ds.completionAccounts[0] == ds.completionAccounts[1] || ds.completionAccounts[1] == ds.completionAccounts[2] || ds.completionAccounts[0] == ds.completionAccounts[2] {
		t.Fatalf("rate limit retried the same owner: %+v accounts=%v", failure, ds.completionAccounts)
	}
	failed, _ := r.Store.FindAccount(old)
	secondFailed, _ := r.Store.FindAccount(ds.completionAccounts[1])
	healthy, _ := r.Store.FindAccount(a.AccountID)
	if !failed.IsCoolingDown() || !secondFailed.IsCoolingDown() || healthy.IsCoolingDown() {
		t.Fatal("cooldown was not bound to the failed account")
	}
}

func TestCooldownRecordedWhenNoReplacementOrAccountIsPinned(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		r, a := restartAuthFixture(t)
		old := a.AccountID
		if pinned {
			a.TargetAccount = old
		} else if err := r.Store.Update(func(c *config.Config) error {
			for i := range c.Accounts {
				if c.Accounts[i].Identifier() != old {
					c.Accounts[i].ManualDisabled = true
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		r.Pool.Reset()
		attempted := false
		failure := &assistantturn.OutputError{Status: 429, Code: "upstream_empty_output"}
		if canRetryOnAlternateAccount(context.Background(), a, failure, true, &attempted) {
			t.Fatal("moved to an unavailable replacement")
		}
		acc, _ := r.Store.FindAccount(old)
		if !acc.IsCoolingDown() || acc.ManualDisabled {
			t.Fatal("failed switch lost cooldown or changed the manual switch")
		}
	}
}

func TestGenericFailuresDoNotCooldownHealthyAccountsAndBansAreDeferred(t *testing.T) {
	r, a := restartAuthFixture(t)
	for _, failure := range []*assistantturn.OutputError{{Status: 503, Code: "upstream_unavailable", Message: "network down"}, {Status: 400, Code: "content_filter"}, {Status: 502, Code: "upstream_prompt_echo"}} {
		if ObserveAccountOutputFailure(a, failure) {
			t.Fatal("generic failure was treated as an account rate limit")
		}
	}
	acc, _ := r.Store.FindAccount(a.AccountID)
	if acc.IsCoolingDown() {
		t.Fatal("healthy account was cooled down")
	}
	ban := &assistantturn.OutputError{Status: 403, Code: "account_banned"}
	if !CanDeferOutputError(ban, true, false) || CanDeferOutputError(ban, true, true) {
		t.Fatal("ban recovery ignores delivery boundaries")
	}
}
