package chat

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/tooldiagnostics"
)

func TestVercelReleaseRetainsOnlyRealToolEvidence(t *testing.T) {
	const tool = `<tool_calls><invoke name="read"></invoke></tool_calls>`
	for _, tc := range []struct {
		name, content, thinking, finish string
		catalog, want, interrupted      bool
	}{
		{"schema-less content", tool, "", "error", false, true, false},
		{"hidden reasoning", "", tool, "error", false, true, false},
		{"catalog prose", "ordinary prose", "", "error", true, false, false},
		{"cancelled candidate", tool, "", "stopped", false, true, true},
		{"cancelled prose", "ordinary prose", "", "stopped", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			std := promptcompat.StandardRequest{ResponseModel: "model"}
			if tc.catalog {
				std.ToolNames = []string{"read"}
			}
			lease := streamLease{Diagnostics: tooldiagnostics.NewRecorder(s, "chat", true), Standard: std}
			req := map[string]any{"content": tc.content, "thinking": tc.thinking, "finish_reason": tc.finish}
			if !tc.interrupted {
				req["error"], req["error_code"] = "stream interrupted", "upstream_interrupted"
			}
			finishLeaseDiagnostics(httptest.NewRequest("POST", "/", nil), lease, req)
			page := s.List(tooldiagnostics.Query{})
			if (page.Total == 1) != tc.want {
				t.Fatalf("wrong relevance: %+v", page)
			}
			if !tc.want {
				return
			}
			record, err := s.Get(page.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			a := record.Attempts[0]
			wantDelivery := "not_delivered"
			if tc.interrupted {
				wantDelivery = "unknown"
			}
			if a.Raw.Text != tool || len(a.Calls) != 1 || a.Calls[0].Delivery != wantDelivery {
				t.Fatalf("lost evidence: %+v", record)
			}
			if tc.thinking != "" && a.Source != "reasoning" {
				t.Fatal("reasoning source lost")
			}
			if tc.interrupted && record.Status != "interrupted" {
				t.Fatal("cancellation lost")
			}
		})
	}
}

func TestVercelEmptyFailureKeepsDiagnosticWithoutToolSyntax(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	std := promptcompat.StandardRequest{ResponseModel: "model", ToolNames: []string{"read"}, ToolsRaw: []any{map[string]any{"type": "function", "name": "read"}}}
	lease := streamLease{Diagnostics: tooldiagnostics.NewRecorder(s, "chat", true), Standard: std}
	finishLeaseDiagnostics(httptest.NewRequest("POST", "/", nil), lease, map[string]any{"finish_reason": "error", "error": "Upstream service is unavailable and returned no output.", "error_code": "upstream_unavailable"})
	page := s.List(tooldiagnostics.Query{})
	if page.Total != 1 || page.Items[0].Status != "failed" {
		t.Fatalf("lost empty-output failure: %+v", page)
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempts[0].Raw.Text != "" || len(record.Attempts[0].Calls) != 0 || record.ErrorCode != "upstream_unavailable" {
		t.Fatalf("empty output was replaced with invented tool evidence: %+v", record)
	}
}
