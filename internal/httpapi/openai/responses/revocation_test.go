package responses

import (
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"testing"
)

func TestRegressionDeletedAPIKeyMustNotRetrieveStoredResponses(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"keys":["revoked-review-key"]}`)
	t.Setenv("DS2API_ENV_WRITEBACK", "")
	s, err := config.LoadStoreWithError()
	if err != nil {
		t.Fatal(err)
	}
	resolver := auth.NewResolver(s, nil, nil)
	request := httptest.NewRequest("GET", "/v1/responses/resp-review", nil)
	request.Header.Set("Authorization", "Bearer revoked-review-key")
	caller, err := resolver.DetermineCaller(request)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Store: s, Auth: resolver}
	h.getResponseStore().putContext(responseStoreOwner(caller), "resp-review", map[string]any{"id": "resp-review", "output": "private content"}, []any{map[string]any{"role": "user", "content": "original"}})
	router := chi.NewRouter()
	router.Get("/v1/responses/{response_id}", h.GetResponseByID)
	before := httptest.NewRecorder()
	router.ServeHTTP(before, request)
	if before.Code != 200 {
		t.Fatalf("active key cannot read its response: %d", before.Code)
	}
	if _, err := h.restoreContinuation(request, map[string]any{"previous_response_id": "resp-review", "input": "continue"}); err != nil {
		t.Fatalf("active continuation failed: %v", err)
	}
	if err = s.Update(func(c *config.Config) error { c.Keys = nil; c.APIKeys = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if s.HasAPIKey("revoked-review-key") {
		t.Fatal("key not deleted")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, request)
	if rec.Code != 404 {
		t.Fatalf("deleted key still retrieves stored response: %s", rec.Body.String())
	}
	if _, err := h.restoreContinuation(request, map[string]any{"previous_response_id": "resp-review", "input": "continue"}); err == nil {
		t.Fatal("deleted key can still continue private history")
	}
}

func TestResponseOwnerRequiresCallerIdentity(t *testing.T) {
	if responseStoreOwner(nil) != "" || responseStoreOwner(&auth.RequestAuth{}) != "" || responseStoreOwner(&auth.RequestAuth{ManagedCredential: true}) != "" {
		t.Fatal("missing caller identity became a usable cache owner")
	}
}
