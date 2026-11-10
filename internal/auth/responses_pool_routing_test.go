package auth

import (
	"net/http/httptest"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestResponsesInstructionsRouteToToolsOnlyAccount(t *testing.T) {
	resolver := newMixedPoolResolver(t)
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer managed-key")
	body := map[string]any{"instructions": "Available tools: read_file(path)", "input": "TASK"}
	req = req.WithContext(WithToolsPresent(req.Context(), promptcompat.ApplyCallerFlags(resolver, req, body, "openai_responses")))
	auth, err := resolver.Determine(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Release(auth)
	if !auth.ToolsPresent || auth.AccountID != "only@example.com" {
		t.Fatal("Responses catalog selected the wrong account pool")
	}
}
