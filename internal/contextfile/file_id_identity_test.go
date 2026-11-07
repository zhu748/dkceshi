package contextfile

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

func TestGeneratedRefsPreserveCaseSensitiveFileIdentities(t *testing.T) {
	got := prependUniqueRefFileIDs([]string{"file-A", "file-a", " file-A "}, "history-X", "history-x")
	if !reflect.DeepEqual(got, []string{"history-X", "history-x", "file-A", "file-a"}) {
		t.Fatalf("distinct opaque IDs were combined: %v", got)
	}
	got = replaceGeneratedCurrentInputRefs([]string{"HISTORY", "history", "TOOLS", "tools", "CLIENT"}, "HISTORY", "TOOLS", "new-history", "new-tools")
	if !reflect.DeepEqual(got, []string{"new-history", "new-tools", "history", "tools", "CLIENT"}) {
		t.Fatalf("reupload removed client-owned case variants: %v", got)
	}
}

func TestCaseVariantClientFilePreventsAccountFailover(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	t.Cleanup(pool.Close)
	r := auth.NewResolver(store, pool, func(_ context.Context, a config.Account) (string, error) { return "token-" + a.Identifier(), nil })
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer key")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Release(a) })
	owner, attempts := a.AccountID, 0
	_, err = retryContextFileAccount(context.Background(), a, promptcompat.StandardRequest{CurrentInputFileID: "generated", RefFileIDs: []string{"GENERATED"}}, func(_ context.Context, _ *auth.RequestAuth, req promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
		attempts++
		if attempts == 1 {
			return req, &dsclient.FileRateLimitError{Op: "upload", Code: 40029}
		}
		return req, nil
	})
	if err == nil || attempts != 1 || a.AccountID != owner {
		t.Fatalf("client-owned ID crossed accounts: attempts=%d owner=%s err=%v", attempts, a.AccountID, err)
	}
}
