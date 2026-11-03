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

type preparationFailoverDS struct {
	poolFailoverDS
	failAt string
}

func (d *preparationFailoverDS) CreateSession(ctx context.Context, a *auth.RequestAuth, n int) (string, error) {
	if !a.AccountPinned() {
		return "", fmt.Errorf("session not pinned")
	}
	if d.failAt == "session" && a.AccountID == d.first {
		return "", d.reject(a)
	}
	return d.inlineUploadDSStub.CreateSession(ctx, a, n)
}
func (d *preparationFailoverDS) GetPow(ctx context.Context, a *auth.RequestAuth, n int) (string, error) {
	if !a.AccountPinned() {
		return "", fmt.Errorf("pow not pinned")
	}
	if d.failAt == "pow" && a.AccountID == d.first {
		return "", d.reject(a)
	}
	return d.inlineUploadDSStub.GetPow(ctx, a, n)
}
func (d *preparationFailoverDS) UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, n int) (*dsclient.UploadFileResult, error) {
	if d.failAt == "history" && a.AccountID == d.first && req.Filename == "HISTORY.txt" {
		return nil, d.reject(a)
	}
	return d.poolFailoverDS.UploadFile(ctx, a, req, n)
}

func TestAttachmentsRebuildWhenLaterPreparationFails(t *testing.T) {
	for _, phase := range []string{"session", "pow", "history"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", phase, stream), func(t *testing.T) {
				t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"input_file":{"enabled":true,"min_chars":1},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
				s := config.LoadStore()
				p := account.NewPool(s)
				defer p.Close()
				r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
				ds := &preparationFailoverDS{poolFailoverDS: poolFailoverDS{resolver: r}, failAt: phase}
				h := &openAITestSurface{Store: s, Auth: r, DS: ds}
				body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash","stream":%v,"messages":[{"role":"user","content":[{"type":"text","text":"continue"},{"type":"input_file","filename":"one.txt","file_data":"QUJDRA=="}]}]}`, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer key")
				rec := httptest.NewRecorder()
				h.ChatCompletions(rec, req)
				if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ok") {
					t.Fatalf("failed preparation recovery: %d %s", rec.Code, rec.Body.String())
				}
				if len(ds.replyOwners) != 1 || ds.replyOwners[0] == ds.first {
					t.Fatalf("wrong completion owner: %v", ds.replyOwners)
				}
				for _, id := range ds.completionReq["ref_file_ids"].([]any) {
					if !strings.HasPrefix(fmt.Sprint(id), ds.replyOwners[0]+"-") {
						t.Fatalf("mixed file owners: %v", id)
					}
				}
				if p.Status()["in_use"] != 0 {
					t.Fatal("leaked slot")
				}
			})
		}
	}
}
