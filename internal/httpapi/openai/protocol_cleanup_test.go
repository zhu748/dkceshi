package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/claude"
	"ds2api/internal/httpapi/gemini"
	"github.com/go-chi/chi/v5"
)

type protocolCleanupDS struct {
	inlineUploadDSStub
	failPow bool
	deletes atomic.Int64
	deleted chan string
}

func (d *protocolCleanupDS) GetPow(context.Context, *auth.RequestAuth, int) (string, error) {
	if d.failPow {
		return "", fmt.Errorf("network unavailable")
	}
	return "pow", nil
}

func (d *protocolCleanupDS) DeleteSession(_ context.Context, owner *auth.RequestAuth, id string, _ int) (*dsclient.DeleteSessionResult, error) {
	d.deletes.Add(1)
	d.deleted <- owner.AccountID + ":" + id
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (d *protocolCleanupDS) DeleteSessionForToken(context.Context, string, string) (*dsclient.DeleteSessionResult, error) {
	d.deletes.Add(1) // A legacy fallback here would duplicate shared cleanup.
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func TestEveryProtocolFinalizesOwnedSessionsOnSuccessAndStartupFailure(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "claude", "gemini"} {
		for _, stream := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%v/failPow=%v", protocol, stream, fail), func(t *testing.T) {
					t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"input_file":{"enabled":false},"auto_delete":{"mode":"single"},"accounts":[{"email":"owner@example.com","token":"token"}]}`)
					store := config.LoadStore()
					pool := account.NewPool(store)
					defer pool.Close()
					resolver := auth.NewResolver(store, pool, func(context.Context, config.Account) (string, error) { return "token", nil })
					ds := &protocolCleanupDS{failPow: fail, deleted: make(chan string, 4)}
					body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash","stream":%v,"messages":[{"role":"user","content":"hi"}]}`, stream)
					var handle http.HandlerFunc
					switch protocol {
					case "chat":
						handle = (&openAITestSurface{Store: store, Auth: resolver, DS: ds}).ChatCompletions
					case "responses":
						body = fmt.Sprintf(`{"model":"deepseek-v4.1-flash","stream":%v,"input":"hi"}`, stream)
						handle = (&openAITestSurface{Store: store, Auth: resolver, DS: ds}).responsesHandler().Responses
					case "claude":
						handle = (&claude.Handler{Store: store, Auth: resolver, DS: ds}).Messages
					case "gemini":
						body = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
						h := &gemini.Handler{Store: store, Auth: resolver, DS: ds}
						handle = h.GenerateContent
						if stream {
							handle = h.StreamGenerateContent
						}
					}
					req := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer key")
					route := chi.NewRouteContext()
					route.URLParams.Add("model", "deepseek-v4.1-flash")
					req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
					rec := httptest.NewRecorder()
					handle(rec, req)
					if fail && rec.Code < 400 || !fail && rec.Code != http.StatusOK {
						t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
					}
					select {
					case deleted := <-ds.deleted:
						if deleted != "owner@example.com:session-id" {
							t.Fatalf("wrong cleanup owner: %s", deleted)
						}
					case <-time.After(time.Second):
						t.Fatal("session cleanup did not run")
					}
					if ds.deletes.Load() != 1 || pool.Status()["in_use"] != 0 {
						t.Fatalf("duplicate cleanup or leaked slot: deletes=%d pool=%v", ds.deletes.Load(), pool.Status())
					}
				})
			}
		}
	}
}
