package contextfile

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type failoverContextUploader struct {
	resolver  *auth.Resolver
	first     string
	banned    bool
	owners    []string
	histories []string
}

func (u *failoverContextUploader) UploadFile(_ context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if !a.AccountPinned() {
		return nil, fmt.Errorf("context upload was not pinned")
	}
	u.owners = append(u.owners, a.AccountID)
	if req.Filename == "HISTORY.txt" {
		u.histories = append(u.histories, string(req.Data))
	}
	if a.AccountID == u.first && req.Filename == "TOOLS.txt" {
		if u.banned {
			u.resolver.SetAccountBanned(a, "USER_IS_BANNED")
			return nil, &dsclient.RequestFailure{Kind: dsclient.FailureBanned}
		}
		return nil, &dsclient.FileRateLimitError{Op: "fetch files", Code: 40029}
	}
	return &dsclient.UploadFileResult{ID: a.AccountID + "-" + req.Filename, AccountID: a.AccountID, Status: "SUCCESS"}, nil
}

func TestContextFilesRebuildTogetherAfterRateLimitOrBan(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		for _, banned := range []bool{false, true} {
			t.Run(fmt.Sprintf("sticky=%v/banned=%v", sticky, banned), func(t *testing.T) {
				t.Setenv("DS2API_CONFIG_JSON", fmt.Sprintf(`{"keys":["key"],"runtime":{"account_sticky_enabled":%v},"input_file":{"enabled":true,"min_chars":1},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`, sticky))
				store := config.LoadStore()
				pool := account.NewPool(store)
				defer pool.Close()
				r := auth.NewResolver(store, pool, func(_ context.Context, a config.Account) (string, error) { return "token-" + a.Identifier(), nil })
				req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
				req.Header.Set("Authorization", "Bearer key")
				a, err := r.Determine(req)
				if err != nil {
					t.Fatal(err)
				}
				defer r.Release(a)
				first := a.AccountID
				u := &failoverContextUploader{resolver: r, first: first, banned: banned}
				standard, err := promptcompat.NormalizeOpenAIChatRequest(store, map[string]any{
					"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "system", "content": "system rules"}, map[string]any{"role": "user", "content": "continue"}},
					"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"type": "object"}}}},
				}, "")
				if err != nil {
					t.Fatal(err)
				}
				out, err := (Service{Store: store, DS: u}).ApplyCurrentInputFile(context.Background(), a, standard)
				if err != nil {
					t.Fatal(err)
				}
				if a.AccountID == first || a.TargetAccount != "" || !out.CurrentInputFileApplied || len(out.RefFileIDs) != 2 || len(u.histories) != 2 || u.histories[0] != u.histories[1] {
					t.Fatalf("failed rebuild: owner=%s out=%+v uploads=%v", a.AccountID, out, u.owners)
				}
				for _, id := range out.RefFileIDs {
					if !strings.HasPrefix(id, a.AccountID+"-") {
						t.Fatal("mixed account file IDs")
					}
				}
				failed, _ := store.FindAccount(first)
				if banned {
					if !failed.Banned || !failed.Disabled {
						t.Fatal("banned account remained enabled")
					}
				} else if !failed.IsCoolingDown() {
					t.Fatal("rate-limited account remained schedulable")
				}
				r.Release(a)
				if pool.Status()["in_use"] != 0 {
					t.Fatal("failover leaked a slot")
				}
			})
		}
	}
}
