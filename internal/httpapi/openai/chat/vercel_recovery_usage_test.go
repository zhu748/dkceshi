package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/promptcompat"
)

type refreshingPowDS struct{ inlineUploadDSStub }

func (d *refreshingPowDS) GetPow(ctx context.Context, a *auth.RequestAuth, _ int) (string, error) {
	a.RecoverAuthFailure(ctx)
	return "new-pow", nil
}

type slowReleaseDeleteDS struct {
	inlineUploadDSStub
	finish chan struct{}
}

func (d *slowReleaseDeleteDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	<-d.finish
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func TestVercelUsageArrivesBeforeSlowRemoteCleanup(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "secret")
	finish := make(chan struct{})
	h := &Handler{Store: mockOpenAIConfig{autoDeleteMode: "single"}, DS: &slowReleaseDeleteDS{finish: finish}}
	id := h.holdStreamLease(&auth.RequestAuth{DeepSeekToken: "token"}, promptcompat.StandardRequest{ResponseModel: "model", PromptTokenText: "history", RefFileTokens: 1000}, "session")
	server := httptest.NewServer(http.HandlerFunc(h.handleVercelStreamRelease))
	defer server.Close()
	defer close(finish)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL, strings.NewReader(`{"lease_id":"`+id+`","usage_accounting_version":1,"content":"answer"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Ds2-Internal-Token", "secret")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	if closeErr := res.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Usage struct {
			Prompt int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Usage.Prompt < 1000 {
		t.Fatalf("usage did not arrive before cleanup: %s", body)
	}
}

func TestVercelPowSynchronizesRefreshedCredentials(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "secret")
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"old-token"}]}`)
	s := config.LoadStore()
	p := account.NewPool(s)
	defer p.Close()
	r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "new-token", nil })
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer key")
	a, err := r.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release(a)
	h := &Handler{DS: &refreshingPowDS{}}
	id := h.holdStreamLease(a, promptcompat.StandardRequest{}, "session")
	req = httptest.NewRequest("POST", "/?__stream_pow=1", strings.NewReader(`{"lease_id":"`+id+`"}`))
	req.Header.Set("X-Ds2-Internal-Token", "secret")
	w := httptest.NewRecorder()
	h.handleVercelStreamPow(w, req)
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["deepseek_token"] != "new-token" || body["pow_header"] != "new-pow" || body["base_headers"] == nil {
		t.Fatalf("credentials not synchronized: %s", w.Body)
	}
}

func TestVercelUsageCountsHistoryFilesAndRetryPrompts(t *testing.T) {
	std := promptcompat.StandardRequest{ResponseModel: "deepseek-v4.1-flash", FinalPrompt: "short live input", PromptTokenText: strings.Repeat("earlier history ", 100), RefFileTokens: 1000}
	req := map[string]any{"usage_accounting_version": 1, "retry_prompts": []any{"actual follow-up"}, "thinking": "plan", "content": "answer"}
	got := finalizeVercelUsage(std, req)
	want := assistantturn.OpenAIChatUsage(assistantturn.Turn{Usage: assistantturn.BuildUsage(std.ResponseModel, std.PromptTokenText+"\nactual follow-up", "plan", "answer", 1000)})
	for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens"} {
		if got[key] != want[key] {
			t.Fatalf("%s got=%v want=%v", key, got, want)
		}
	}
	if req["usage"] == nil {
		t.Fatal("ledger must receive the same authoritative usage")
	}
	delete(req, "usage_accounting_version")
	if finalizeVercelUsage(std, req) != nil {
		t.Fatal("legacy bridge accounting must remain compatible")
	}
	if finalizeVercelUsage(std, map[string]any{"usage_accounting_version": 1}) != nil {
		t.Fatal("prepare-only lease must not produce consumption")
	}
}

func TestVercelValidationCountsReferencesAndRetries(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "secret")
	h := &Handler{DS: &inlineUploadDSStub{}}
	std := promptcompat.StandardRequest{ResponseModel: "deepseek-v4.1-flash", PromptTokenText: "hello", RefFileTokens: 1000}
	id := h.holdStreamLease(&auth.RequestAuth{}, std, "session")
	r := httptest.NewRequest("POST", "/?__stream_validate=1", strings.NewReader(`{"lease_id":"`+id+`","content":"answer","retry_prompts":["follow-up"]}`))
	r.Header.Set("X-Ds2-Internal-Token", "secret")
	w := httptest.NewRecorder()
	h.handleVercelStreamValidate(w, r)
	var body struct {
		Usage struct {
			Prompt int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := assistantturn.BuildUsage(std.ResponseModel, "hello\nfollow-up", "", "answer", 1000).InputTokens
	if body.Usage.Prompt != want {
		t.Fatalf("got=%d want=%d response=%s", body.Usage.Prompt, want, w.Body)
	}
}
