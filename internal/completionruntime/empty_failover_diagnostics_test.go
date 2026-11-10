package completionruntime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
)

func TestEmptyStreamFailoverKeepsEveryAttemptAndStopReason(t *testing.T) {
	for _, mode := range []string{"healthy_fourth", "budget", "explicit_target", "no_replacement"} {
		t.Run(mode, func(t *testing.T) {
			r, a := restartAuthFixture(t)
			if err := r.Store.Update(func(c *config.Config) error {
				for i := 0; i < 2; i++ {
					c.Accounts = append(c.Accounts, config.Account{Email: fmt.Sprintf("extra%d@example.com", i), Token: "token"})
				}
				if mode == "no_replacement" {
					for i := range c.Accounts {
						c.Accounts[i].ManualDisabled = c.Accounts[i].Identifier() != a.AccountID
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			r.Pool.Reset()
			if mode == "explicit_target" {
				a.TargetAccount = a.AccountID
			}
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			recorder := tooldiagnostics.NewRecorder(store, "chat", true)
			ctx := tooldiagnostics.WithRecorder(context.Background(), recorder)
			std := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", ResponseModel: "test", FinalPrompt: "original context", ToolNames: []string{"read"}, ToolsRaw: []any{map[string]any{"type": "function", "name": "read"}}}
			empty := func() *http.Response { return sseHTTPResponse(200, "data: [DONE]") }
			ds := &fakeDeepSeekCaller{sessionByAccount: true, responses: []*http.Response{empty(), empty(), empty()}}
			if mode == "healthy_fourth" {
				ds.responses[2] = sseHTTPResponse(200, `data: {"p":"response/content","v":"healthy answer"}`, "data: [DONE]")
			}
			seen := []string{}
			var turn assistantturn.Turn
			var failure *assistantturn.OutputError
			terminalRetries := -1
			ExecuteStreamWithRetry(ctx, ds, a, empty(), std.CompletionPayload("initial"), "pow", StreamRetryOptions{Request: std, RetryEnabled: true}, StreamRetryHooks{
				ConsumeAttempt: func(resp *http.Response, deferEmpty bool) ConsumeAttemptResult {
					seen = append(seen, a.AccountID)
					var err *assistantturn.OutputError
					turn, err = collectAttempt(ctx, resp, a, std, std.FinalPrompt, Options{diagnosticCtx: ctx})
					if err != nil {
						t.Fatal(err)
					}
					turn = assistantturn.BuildTurnFromCollected(sse.CollectResult{Text: turn.RawText, Thinking: turn.RawThinking}, buildOptions(std, std.FinalPrompt, Options{diagnosticCtx: ctx}))
					if turn.Error != nil && !deferEmpty {
						t.Fatal("empty output reached the terminal before failover")
					}
					if turn.Error == nil {
						tooldiagnostics.ResponseWritten(ctx)
					}
					return ConsumeAttemptResult{Terminal: turn.Error == nil, Retryable: turn.Error != nil}
				},
				ResponseCompleted: func() bool { return turn.Error == nil },
				EmptyOutputError:  func() *assistantturn.OutputError { return turn.Error },
				OnRetryFailure: func(status int, message, code string) {
					failure = &assistantturn.OutputError{Status: status, Message: message, Code: code}
				},
				OnTerminal: func(retries int) { terminalRetries = retries },
			})
			recorder.Finish(200, nil, false)
			page := store.List(tooldiagnostics.Query{})
			if page.Total != 1 {
				t.Fatalf("empty attempt lost from diagnostics: %+v", page)
			}
			record, err := store.Get(page.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			wantAttempts, wantStop := 4, ""
			switch mode {
			case "healthy_fourth":
				if failure != nil || terminalRetries != 3 || record.Status != "regenerated_success" {
					t.Fatalf("healthy account not recovered or retries misreported: failure=%+v retries=%d status=%s", failure, terminalRetries, record.Status)
				}
			case "budget":
				wantStop = "account_switch_budget_exhausted"
			case "explicit_target":
				wantAttempts, wantStop = 1, "explicit_target"
			case "no_replacement":
				wantAttempts, wantStop = 1, "no_eligible_account"
			}
			if len(seen) != wantAttempts || len(record.Attempts) != wantAttempts {
				t.Fatalf("wrong attempts: consumed=%d recorded=%d want=%d", len(seen), len(record.Attempts), wantAttempts)
			}
			if wantStop != "" && (failure == nil || failure.Code != "upstream_unavailable" || record.Attempts[len(record.Attempts)-1].RetryDecision != wantStop) {
				t.Fatalf("stop reason lost: failure=%+v last=%+v", failure, record.Attempts[len(record.Attempts)-1])
			}
			unique := map[string]bool{}
			for i, id := range seen {
				if unique[id] || record.Attempts[i].AccountID != id {
					t.Fatal("same failed account retried or diagnostic owner changed")
				}
				unique[id] = true
				acc, _ := r.Store.FindAccount(id)
				if (mode != "healthy_fourth" || i < len(seen)-1) && !acc.IsCoolingDown() {
					t.Fatal("empty account did not cool down")
				}
			}
			for _, payload := range ds.payloads {
				if strings.Contains(payload["prompt"].(string), EmptyOutputRetrySuffix) || payload["parent_message_id"] != nil {
					t.Fatal("replacement reused rejected follow-up state")
				}
			}
		})
	}
}

func TestNonStreamEmptyFailoverCountsAccountReplacement(t *testing.T) {
	_, a := restartAuthFixture(t)
	ds := &fakeDeepSeekCaller{responses: []*http.Response{sseHTTPResponse(200, "data: [DONE]"), sseHTTPResponse(200, `data: {"p":"response/content","v":"healthy"}`, "data: [DONE]")}}
	result, err := ExecuteNonStreamWithRetry(context.Background(), ds, a, promptcompat.StandardRequest{FinalPrompt: "task"}, Options{RetryEnabled: true})
	if err != nil || result.Attempts != 1 || len(ds.completionAccounts) != 2 || ds.completionAccounts[0] == ds.completionAccounts[1] {
		t.Fatalf("lost failover: err=%+v attempts=%d accounts=%v", err, result.Attempts, ds.completionAccounts)
	}
}

func TestStartupFailoverRetainsReplacementsThatNeverOpenedAStream(t *testing.T) {
	_, a := restartAuthFixture(t)
	first := a.AccountID
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	recorder := tooldiagnostics.NewRecorder(store, "chat", false)
	ctx := tooldiagnostics.WithRecorder(context.Background(), recorder)
	ds := &fakeDeepSeekCaller{sessionErrors: []error{errors.New("initial session unavailable"), errors.New("replacement session unavailable")}}
	std := promptcompat.StandardRequest{ResponseModel: "test", FinalPrompt: "task", ToolNames: []string{"read"}, ToolsRaw: []any{map[string]any{"type": "function", "name": "read"}}}
	_, outErr := ExecuteNonStreamWithRetry(ctx, ds, a, std, Options{RetryEnabled: true})
	if outErr != nil {
		t.Fatal(outErr)
	}
	recorder.Finish(200, nil, false)
	page := store.List(tooldiagnostics.Query{})
	if page.Total != 1 {
		t.Fatalf("lost startup diagnostics: %+v", page)
	}
	record, err := store.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Attempts) != 3 || len(ds.payloads) != 1 || record.Status != "regenerated_success" {
		t.Fatalf("unopened replacement was not kept: attempts=%d completions=%d status=%s", len(record.Attempts), len(ds.payloads), record.Status)
	}
	if record.Attempts[0].AccountID != first || record.Attempts[2].AccountID != a.AccountID || record.Attempts[0].RetryDecision != "started" || record.Attempts[1].RetryDecision != "started" {
		t.Fatalf("wrong startup owners or decisions: %+v", record.Attempts)
	}
}
