package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

// Uploads succeed; the rejection occurs only after the prepared attachment IDs
// have replaced the original bytes, exercising the full request replay boundary.
func TestAttachmentFailoverAfterSuccessfulUpload(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, split := range []bool{false, true} {
				for _, banned := range []bool{false, true} {
					t.Run(fmt.Sprintf("sticky=%v/stream=%v/split=%v/banned=%v", sticky, stream, split, banned), func(t *testing.T) {
						t.Setenv("DS2API_CONFIG_JSON", fmt.Sprintf(`{"keys":["key"],"runtime":{"account_sticky_enabled":%v},"input_file":{"enabled":%v,"min_chars":1},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`, sticky, split))
						s := config.LoadStore()
						p := account.NewPool(s)
						defer p.Close()
						r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
						ds := &poolFailoverDS{resolver: r, phase: "completion", banned: banned}
						h := &openAITestSurface{Store: s, Auth: r, DS: ds}
						body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash","stream":%v,"messages":[{"role":"user","content":[{"type":"text","text":"continue"},{"type":"input_file","filename":"one.txt","file_data":"QUJDRA=="}]}]}`, stream)
						req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
						req.Header.Set("Authorization", "Bearer key")
						rec := httptest.NewRecorder()
						h.ChatCompletions(rec, req)
						if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ok") {
							t.Fatalf("failed recovery: %d %s", rec.Code, rec.Body.String())
						}
						if len(ds.replyOwners) != 2 || ds.replyOwners[0] == ds.replyOwners[1] {
							t.Fatalf("wrong owners: %v", ds.replyOwners)
						}
						refs, _ := ds.completionReq["ref_file_ids"].([]any)
						for _, id := range refs {
							if !strings.HasPrefix(fmt.Sprint(id), ds.replyOwners[1]+"-") {
								t.Fatalf("stale attachment: %v", refs)
							}
						}
						if strings.Contains(fmt.Sprint(ds.completionReq["prompt"]), ds.first+"-one.txt") {
							t.Fatal("old file ID survived in prompt")
						}
						if p.Status()["in_use"] != 0 {
							t.Fatal("leaked account slot")
						}
					})
				}
			}
		}
	}
}

type contextAuthFailoverDS struct {
	inlineUploadDSStub
	first string
}

func (d *contextAuthFailoverDS) UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if d.first == "" {
		d.first = a.AccountID
	}
	if a.AccountID == d.first {
		a.RecoverAuthFailure(ctx)
		return nil, &dsclient.RequestFailure{Kind: dsclient.FailureManagedUnauthorized, Message: "rejected after refresh"}
	}
	return &dsclient.UploadFileResult{ID: a.AccountID + req.Filename, AccountID: a.AccountID, Status: "SUCCESS"}, nil
}
func TestContextAuthFailureUsesHealthyAccountWithoutCooldown(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"input_file":{"enabled":true,"min_chars":1},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
	s := config.LoadStore()
	p := account.NewPool(s)
	defer p.Close()
	logins := map[string]int{}
	r := auth.NewResolver(s, p, func(_ context.Context, a config.Account) (string, error) {
		logins[a.Identifier()]++
		if logins[a.Identifier()] > 1 {
			return "", auth.ErrLoginRejected
		}
		return "token", nil
	})
	ds := &contextAuthFailoverDS{}
	h := &openAITestSurface{Store: s, Auth: r, DS: ds}
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"continue"}]}`))
	req.Header.Set("Authorization", "Bearer key")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)
	if rec.Code != 200 {
		t.Fatalf("auth failure leaked: %d %s", rec.Code, rec.Body.String())
	}
	acc, _ := s.FindAccount(ds.first)
	if acc.IsCoolingDown() || acc.Banned || acc.AuthFailed {
		t.Fatalf("incorrect account classification: %+v", acc)
	}
	if p.Status()["in_use"] != 0 {
		t.Fatal("leaked slot")
	}
}
