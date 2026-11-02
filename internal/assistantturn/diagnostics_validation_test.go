package assistantturn

import (
	"context"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
	"path/filepath"
	"testing"
)

func TestDiagnosticsReuseValidationAndRespectPolicySkips(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "name": "read", "parameters": map[string]any{"type": "object", "required": []any{"path"}, "properties": map[string]any{"path": map[string]any{"type": "string"}}}}}
	const raw = `<tool_calls><invoke name="read"></invoke><invoke name="read"><parameter name="path">file.txt</parameter></invoke></tool_calls>`
	for _, policy := range []promptcompat.ToolChoicePolicy{promptcompat.DefaultToolChoicePolicy(), {Mode: promptcompat.ToolChoiceNone}} {
		s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
		r := tooldiagnostics.NewRecorder(s, "chat", false)
		ctx := tooldiagnostics.WithRecorder(context.Background(), r)
		turn := BuildTurnFromCollected(sse.CollectResult{Text: raw}, BuildOptions{Model: "model", ToolsRaw: tools, ToolChoice: policy, DiagnosticContext: ctx})
		if turn.Error == nil {
			t.Fatal("invalid batch accepted")
		}
		r.Finish(turn.Error.Status, nil, false)
		page := s.List(tooldiagnostics.Query{})
		if page.Total != 1 {
			t.Fatal("missing record")
		}
		record, err := s.Get(page.Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		calls := record.Attempts[0].Calls
		if policy.IsNone() {
			if len(turn.ToolArgumentValidation) != 0 || calls[0].Schema != "skipped" || calls[1].Schema != "skipped" {
				t.Fatal("invented validation after policy rejection")
			}
		} else if len(turn.ToolArgumentValidation) != 2 || calls[0].Schema != "failed" || calls[1].Schema != "passed" {
			t.Fatalf("missing actual outcomes: %+v", calls)
		}
	}
}
