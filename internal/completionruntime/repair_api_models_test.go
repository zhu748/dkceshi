package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestRepairModelDiscoveryUsesNativeAuthPaginationAndGenerationFilter(t *testing.T) {
	for _, protocol := range []string{"openai", "claude", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page := requests.Add(1)
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/models") {
					t.Error("wrong discovery route")
				}
				var body any
				switch protocol {
				case "openai":
					if r.Header.Get("Authorization") != "Bearer model-key" {
						t.Error("wrong OpenAI auth")
					}
					body = map[string]any{"data": []any{map[string]any{"id": "a"}, map[string]any{"id": "a"}, map[string]any{"id": "b"}}}
				case "claude":
					if r.Header.Get("x-api-key") != "model-key" || r.Header.Get("anthropic-version") == "" {
						t.Error("wrong Claude auth")
					}
					if page == 1 {
						body = map[string]any{"data": []any{map[string]any{"id": "a", "display_name": "Model A"}}, "has_more": true, "last_id": "a"}
					} else {
						if r.URL.Query().Get("after_id") != "a" {
							t.Error("Claude cursor lost")
						}
						body = map[string]any{"data": []any{map[string]any{"id": "b"}}, "has_more": false}
					}
				case "gemini":
					if r.Header.Get("x-goog-api-key") != "model-key" || r.URL.Query().Get("key") != "" {
						t.Error("wrong Gemini auth")
					}
					if page == 1 {
						body = map[string]any{"models": []any{map[string]any{"name": "models/a", "displayName": "Model A", "supportedGenerationMethods": []string{"generateContent"}}, map[string]any{"name": "models/embedding", "supportedGenerationMethods": []string{"embedContent"}}}, "nextPageToken": "next"}
					} else {
						if r.URL.Query().Get("pageToken") != "next" {
							t.Error("Gemini cursor lost")
						}
						body = map[string]any{"models": []any{map[string]any{"name": "models/b", "supportedGenerationMethods": []string{"generateContent"}}}}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			models, err := ListRepairAPIModels(context.Background(), tooldiagnostics.RepairSettings{Protocol: protocol, BaseURL: server.URL, APIKey: "model-key", TimeoutSeconds: 5})
			if err != nil || len(models.Models) != 2 || models.Models[0].ID != "a" || models.Models[1].ID != "b" || models.Truncated {
				t.Fatalf("model discovery failed: %+v %v", models, err)
			}
			want := int32(2)
			if protocol == "openai" {
				want = 1
			}
			if requests.Load() != want {
				t.Fatal("unexpected pagination requests")
			}
		})
	}
}

func TestRepairModelDiscoveryStopsRepeatedCursorsAndScrubsErrors(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "repeated cursor", true: "error redaction"}[failure], func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if failure {
					w.WriteHeader(401)
					if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "bad secret-model-key"}}); err != nil {
						t.Error(err)
					}
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "a"}}, "has_more": true, "last_id": "same"}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			_, err := ListRepairAPIModels(context.Background(), tooldiagnostics.RepairSettings{Protocol: "claude", BaseURL: server.URL, APIKey: "secret-model-key"})
			if err == nil || strings.Contains(err.Error(), "secret-model-key") {
				t.Fatal("pagination or credential isolation failed")
			}
			if requests.Load() > 2 {
				t.Fatal("pagination did not stop")
			}
		})
	}
}

func TestRepairModelDiscoveryKeepsModelNamesContainingShortDevelopmentKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "local-model"}, map[string]any{"id": "local"}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	result, err := ListRepairAPIModels(context.Background(), tooldiagnostics.RepairSettings{Protocol: "openai", BaseURL: server.URL, APIKey: "local"})
	if err != nil || len(result.Models) != 1 || result.Models[0].ID != "local-model" {
		t.Fatalf("ordinary model names were hidden: %+v %v", result, err)
	}
}
