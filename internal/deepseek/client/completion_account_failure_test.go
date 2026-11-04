package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestCompletionAccountPolicyRecognizesLimitsAndBansWithoutBanningGeneric403(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		body            string
		limited, banned bool
	}{
		{"business rate limit", 200, `{"code":40029,"msg":"TOO_MANY_REQUESTS"}`, true, false},
		{"HTTP rate limit", 429, `slow down`, true, false},
		{"banned JSON", 200, `{"code":0,"data":{"biz_code":10,"biz_msg":"USER_IS_BANNED"}}`, false, true},
		{"banned HTTP", 403, `{"code":0,"data":{"biz_code":10,"biz_msg":"USER_IS_BANNED"}}`, false, true},
		{"other forbidden", 403, `{"code":1,"msg":"access denied"}`, false, false},
		{"normal SSE", 200, "data: {\"v\":\"hello\"}\n\ndata: [DONE]\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
			store := config.LoadStore()
			pool := account.NewPool(store)
			defer pool.Close()
			r := auth.NewResolver(store, pool, func(context.Context, config.Account) (string, error) { return "token", nil })
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			req.Header.Set("Authorization", "Bearer key")
			a, err := r.Determine(req)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Release(a)
			c := &Client{Store: store, Auth: r}
			resp := &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}
			err = c.inspectCompletionFailure(context.Background(), a, resp, "test")
			var limited *FileRateLimitError
			if errors.As(err, &limited) != tc.limited || IsBannedError(err) != tc.banned {
				t.Fatalf("wrong classification: %v", err)
			}
			body, readErr := io.ReadAll(resp.Body)
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if readErr != nil || string(body) != tc.body {
				t.Fatal("inspection consumed response data")
			}
			stored, _ := store.FindAccount(a.AccountID)
			if stored.Banned != tc.banned || stored.IsCoolingDown() != tc.limited {
				t.Fatalf("wrong scheduling state: %+v", stored)
			}
		})
	}
}
