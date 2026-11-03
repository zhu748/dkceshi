package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type poolFailoverDS struct {
	inlineUploadDSStub
	resolver     *auth.Resolver
	first, phase string
	banned       bool
	allEmpty     bool
	replyOwners  []string
	owners       []string
}

func (d *poolFailoverDS) reject(a *auth.RequestAuth) error {
	if d.banned {
		d.resolver.SetAccountBanned(a, "USER_IS_BANNED")
		return &dsclient.RequestFailure{Kind: dsclient.FailureBanned}
	}
	return &dsclient.FileRateLimitError{Op: d.phase, Code: 40029}
}
func (d *poolFailoverDS) UploadFile(_ context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if d.first == "" {
		d.first = a.AccountID
	}
	d.owners = append(d.owners, a.AccountID)
	if d.phase == "inline" && a.AccountID == d.first && req.Filename == "two.txt" {
		return nil, d.reject(a)
	}
	return &dsclient.UploadFileResult{ID: a.AccountID + "-" + req.Filename, AccountID: a.AccountID, Status: "SUCCESS"}, nil
}
func (d *poolFailoverDS) CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, pow string, attempts int) (*http.Response, error) {
	d.replyOwners = append(d.replyOwners, a.AccountID)
	if d.first == "" {
		d.first = a.AccountID
	}
	if d.phase == "empty503" && (a.AccountID == d.first || d.allEmpty) {
		return makeOpenAISSEHTTPResponse(`data: [DONE]`), nil
	}
	if d.phase == "completion" && a.AccountID == d.first {
		return nil, d.reject(a)
	}
	return d.inlineUploadDSStub.CallCompletion(ctx, a, payload, pow, attempts)
}

func TestManagedRequestsFailOverBeforeDeliveringRateLimitsOrBans(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		for _, banned := range []bool{false, true} {
			for _, phase := range []string{"inline", "completion"} {
				t.Run(fmt.Sprintf("sticky=%v/banned=%v/%s", sticky, banned, phase), func(t *testing.T) {
					t.Setenv("DS2API_CONFIG_JSON", fmt.Sprintf(`{"keys":["key"],"runtime":{"account_sticky_enabled":%v},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`, sticky))
					store := config.LoadStore()
					pool := account.NewPool(store)
					defer pool.Close()
					r := auth.NewResolver(store, pool, func(_ context.Context, a config.Account) (string, error) { return "token-" + a.Identifier(), nil })
					ds := &poolFailoverDS{resolver: r, phase: phase, banned: banned}
					h := &openAITestSurface{Store: store, Auth: r, DS: ds}
					content := `"continue"`
					if phase == "inline" {
						content = `[{"type":"text","text":"continue"},{"type":"input_file","filename":"one.txt","file_data":"QUJDRA=="},{"type":"input_file","filename":"two.txt","file_data":"RUZHSA=="}]`
					}
					req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":%s}]}`, content)))
					req.Header.Set("Authorization", "Bearer key")
					rec := httptest.NewRecorder()
					h.ChatCompletions(rec, req)
					if rec.Code != 200 {
						t.Fatalf("failover was passed through: %d %s", rec.Code, rec.Body.String())
					}
					if phase == "inline" {
						if len(ds.owners) != 4 || ds.owners[0] != ds.owners[1] || ds.owners[0] == ds.owners[2] || ds.owners[2] != ds.owners[3] {
							t.Fatalf("partial batch not rebuilt: %v", ds.owners)
						}
						ids, _ := ds.completionReq["ref_file_ids"].([]any)
						if len(ids) != 2 {
							t.Fatalf("unexpected file refs: %v", ids)
						}
						for _, id := range ids {
							if strings.HasPrefix(fmt.Sprint(id), ds.first+"-") {
								t.Fatal("mixed file owners")
							}
						}
					}
					failed, _ := store.FindAccount(ds.first)
					if banned {
						if !failed.Banned || !failed.Disabled {
							t.Fatal("ban was not persisted")
						}
					} else if !failed.IsCoolingDown() {
						t.Fatal("rate-limit cooldown missing")
					}
					if pool.Status()["in_use"] != 0 {
						t.Fatal("request leaked account slot")
					}
				})
			}
		}
	}
}

func TestUnavailableEmptyOutputCoolsAccountAndSwitchesWithoutSameAccountRetry(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, allEmpty := range []bool{false, true} {
				t.Run(fmt.Sprintf("sticky=%v/stream=%v/exhausted=%v", sticky, stream, allEmpty), func(t *testing.T) {
					t.Setenv("DS2API_CONFIG_JSON", fmt.Sprintf(`{"keys":["key"],"runtime":{"account_sticky_enabled":%v},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`, sticky))
					store := config.LoadStore()
					pool := account.NewPool(store)
					defer pool.Close()
					r := auth.NewResolver(store, pool, func(_ context.Context, a config.Account) (string, error) { return "token-" + a.Identifier(), nil })
					ds := &poolFailoverDS{resolver: r, phase: "empty503", allEmpty: allEmpty}
					h := &openAITestSurface{Store: store, Auth: r, DS: ds}
					req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"continue"}],"stream":%v}`, stream)))
					req.Header.Set("Authorization", "Bearer key")
					rec := httptest.NewRecorder()
					before := time.Now()
					h.ChatCompletions(rec, req)
					if len(ds.replyOwners) != 2 || ds.replyOwners[0] == ds.replyOwners[1] {
						t.Fatalf("did not switch directly: %v", ds.replyOwners)
					}
					body := rec.Body.String()
					if !allEmpty && (rec.Code != 200 || strings.Contains(body, "returned no output") || !strings.Contains(body, "ok")) {
						t.Fatalf("did not deliver recovered reply: %d %s", rec.Code, body)
					}
					if allEmpty && !strings.Contains(body, "Upstream service is unavailable and returned no output.") {
						t.Fatalf("lost terminal error: %s", body)
					}
					for i, id := range ds.replyOwners {
						acc, _ := store.FindAccount(id)
						if i == 0 || allEmpty {
							remaining := time.Unix(int64(acc.CooldownUntil), 0).Sub(before)
							if remaining < 119*time.Second || remaining > 121*time.Second || acc.Banned || acc.Disabled {
								t.Fatalf("incorrect temporary cooldown: %+v duration=%v", acc, remaining)
							}
						} else if acc.IsCoolingDown() {
							t.Fatal("healthy replacement was cooled down")
						}
					}
					if pool.Status()["in_use"] != 0 {
						t.Fatal("failover leaked account slots")
					}
				})
			}
		}
	}
}
