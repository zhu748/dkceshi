package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

type routingGeminiAuth struct {
	*auth.Resolver
	selected *auth.RequestAuth
}

func (a *routingGeminiAuth) Determine(r *http.Request) (*auth.RequestAuth, error) {
	selected, err := a.Resolver.Determine(r)
	a.selected = selected
	return selected, err
}

func TestGeminiNativeRoutesNormalizedToolCatalog(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		empty   bool
		present bool
	}{
		{"catalog", "Available tools: read_file", false, true},
		{"ordinary instruction", "Do not call tools. Answer in plain text.", false, false},
		{"explicit empty catalog", "Available tools: read_file", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DS2API_CONFIG_JSON", `{"keys":["routing-test-key"],"accounts":[{"email":"tools@example.com","token":"test-tools","pool_type":"tools_only"},{"email":"plain@example.com","token":"test-plain","pool_type":"no_tools"}]}`)
			store := config.LoadStore()
			a := &routingGeminiAuth{Resolver: auth.NewResolver(store, account.NewPool(store), func(context.Context, config.Account) (string, error) {
				return "routing-test-token", nil
			})}
			h := &Handler{
				Store: testGeminiThinkingConfig{},
				Auth:  a,
				DS: &testGeminiDS{resp: makeGeminiUpstreamResponse(
					`data: {"p":"response/content","v":"ok"}`, "data: [DONE]",
				)},
			}
			body := map[string]any{
				"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": tc.text}}},
				"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}},
			}
			if tc.empty {
				body["tools"] = []any{}
			}
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-flash:generateContent", strings.NewReader(string(data)))
			req.Header.Set("Authorization", "Bearer routing-test-key")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router := chi.NewRouter()
			RegisterRoutes(router, h)
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			wantAccount := "plain@example.com"
			if tc.present {
				wantAccount = "tools@example.com"
			}
			if a.selected == nil || a.selected.ToolsPresent != tc.present || a.selected.AccountID != wantAccount {
				t.Fatalf("expected tools=%v account=%s; selected=%+v", tc.present, wantAccount, a.selected)
			}
		})
	}
}
