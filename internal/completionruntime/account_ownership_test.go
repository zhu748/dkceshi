package completionruntime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type ownershipDS struct {
	fakeDeepSeekCaller
	powCalls int
	moved    bool
}

func (d *ownershipDS) GetPow(ctx context.Context, a *auth.RequestAuth, _ int) (string, error) {
	d.powCalls++
	if d.powCalls == 2 {
		d.moved = a.SwitchAccount(ctx)
	}
	return "pow", nil
}
func TestAlternateStartPinsSessionAndFilesTogether(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"},{"email":"c@example.com","token":"tc"}]}`)
	s := config.LoadStore()
	p := account.NewPool(s)
	defer p.Close()
	r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer key")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release(a)
	d := &ownershipDS{fakeDeepSeekCaller: fakeDeepSeekCaller{sessionByAccount: true, completionErrors: []error{&dsclient.FileRateLimitError{Code: 40029}}}}
	std := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "continue", HistoryText: "context", CurrentInputFileApplied: true, CurrentInputFileID: "original", RefFileIDs: []string{"original"}}
	start, outErr := StartCompletion(context.Background(), d, a, std, Options{CurrentInputFile: currentInputRuntimeConfig{}, RetryEnabled: true})
	if outErr != nil {
		t.Fatal(outErr)
	}
	closeRetryBody("test", start.Response.Body)
	d.powCalls = 1
	if _, err := GetSessionPow(context.Background(), d, a, 3); err != nil {
		t.Fatal(err)
	}
	if d.moved || start.Payload["chat_session_id"] != "session-"+a.AccountID {
		t.Fatalf("mixed resources: owner=%s payload=%v", a.AccountID, start.Payload)
	}
	for _, ref := range start.Payload["ref_file_ids"].([]any) {
		if fmt.Sprint(ref) != "file-runtime-"+a.AccountID {
			t.Fatalf("stale file: %v", ref)
		}
	}
}

type retryPowFailureDS struct {
	fakeDeepSeekCaller
	powCalls int
}

func (d *retryPowFailureDS) GetPow(_ context.Context, a *auth.RequestAuth, _ int) (string, error) {
	d.powCalls++
	if d.powCalls == 2 {
		return "", &dsclient.FileRateLimitError{Op: "pow", Code: 40029}
	}
	return "pow", nil
}
func TestRetryPowRateLimitDoesNotSendOldPayload(t *testing.T) {
	d := &retryPowFailureDS{}
	d.powCalls = 1
	_, outErr := retryPowForAccount(context.Background(), d, &auth.RequestAuth{}, "old", 3)
	if outErr == nil || outErr.Status != http.StatusTooManyRequests {
		t.Fatalf("lost retry failure: %+v", outErr)
	}
	if len(d.payloads) > 0 {
		t.Fatal("sent stale proof")
	}
}
