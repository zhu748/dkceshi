package configmgmt

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func TestManagedKeyToolToggleControlsGeneratedPrompt(t *testing.T) {
	h := newAdminTestHandler(t, `{"api_keys":[{"key":"plain-key"}]}`)
	store, ok := h.Store.(*config.Store)
	if !ok {
		t.Fatal("test requires a real config store")
	}
	router := chi.NewRouter()
	router.Post("/admin/keys", h.addKey)
	router.Put("/admin/keys/{key}", h.updateKey)
	router.Get("/admin/config", h.getConfig)
	resolver := auth.NewResolver(store, nil, nil)

	for i, disabled := range []bool{true, false} {
		method, path, body := http.MethodPut, "/admin/keys/tool-key", `{"force_disable_tools":false}`
		if i == 0 {
			method, path, body = http.MethodPost, "/admin/keys", `{"key":"tool-key","force_disable_tools":true}`
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("save toggle: status=%d body=%s", rec.Code, rec.Body.String())
		}

		caller := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		caller.Header.Set("Authorization", "Bearer tool-key")
		var request map[string]any
		if err := json.Unmarshal([]byte(`{
			"model":"deepseek-v4.1-flash",
			"messages":[{"role":"user","content":"help"}],
			"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]
		}`), &request); err != nil {
			t.Fatal(err)
		}
		if present := promptcompat.ApplyCallerFlags(resolver, caller, request); present == disabled {
			t.Fatalf("tools present=%v with disabled=%v", present, disabled)
		}
		standard, err := promptcompat.NormalizeOpenAIChatRequest(store, request, "")
		if err != nil {
			t.Fatal(err)
		}
		if injected := strings.Contains(standard.FinalPrompt, "<|EPSE|tool_calls>"); injected == disabled {
			t.Fatalf("tool format injected=%v with disabled=%v", injected, disabled)
		}
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/config", nil))
	var body struct {
		Specs []struct {
			Name  string `json:"name"`
			Label string `json:"label_key"`
		} `json:"credential_flag_specs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, spec := range body.Specs {
		if spec.Name == config.CredentialFlagForceDisableTools && spec.Label != "" {
			return
		}
	}
	t.Fatal("the account page must receive metadata for the tool-disable switch")
}
