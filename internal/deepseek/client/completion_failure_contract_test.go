package client

import (
	"context"
	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCompletionFailureContract(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/completion-account-failures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Status int
		Body   json.RawMessage
		Kind   string
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"accounts":[{"email":"a@example.com","token":"token"}]}`)
			s := config.LoadStore()
			p := account.NewPool(s)
			defer p.Close()
			r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
			req := httptest.NewRequest("POST", "/", nil)
			req.Header.Set("Authorization", "Bearer key")
			a, err := r.Determine(req)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Release(a)
			c := &Client{Store: s, Auth: r}
			response := &http.Response{StatusCode: tc.Status, Body: io.NopCloser(strings.NewReader(string(tc.Body)))}
			err = c.inspectCompletionFailure(a, response, "contract")
			if closeErr := response.Body.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			kind := "passthrough"
			var limited *FileRateLimitError
			switch {
			case IsManagedUnauthorizedError(err):
				kind = "unauthorized"
			case errors.As(err, &limited):
				kind = "limited"
			case IsBannedError(err):
				kind = "banned"
			case IsMutedError(err):
				kind = "muted"
			}
			if kind != tc.Kind {
				t.Fatalf("kind=%s want=%s err=%v", kind, tc.Kind, err)
			}
		})
	}
}
