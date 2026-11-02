package assistantturn

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/sse"
	"ds2api/internal/tooldiagnostics"
)

func TestDiagnosticsClassifiesActualRecoveryAndIntegrityRejection(t *testing.T) {
	good := `<|EPSE|tool_calls><|EPSE|invoke name="read"><|EPSE|parameter name="path">a.txt</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	bad := `<|MISTYPED|broken><|MISTYPED|invoke name="read"><|MISTYPED|parameter name="path">a.txt</|MISTYPED|parameter></|MISTYPED|invoke></|MISTYPED|broken>`
	for _, tc := range []struct {
		name, raw, want string
		repair, change  bool
	}{
		{"direct", good, "direct_success", false, false},
		{"shorthand", strings.Replace(good, `parameter name="path"`, `path"`, 1), "local_recovery_success", false, false},
		{"repair", bad, "llm_repair_success", true, false},
		{"changed_identity", bad, "failed", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
			recorder := tooldiagnostics.NewRecorder(store, "chat", false)
			ctx := tooldiagnostics.WithRecorder(context.Background(), recorder)
			opts := BuildOptions{Model: "test", ToolNames: []string{"read"}, DiagnosticContext: ctx, ToolCallRepairCtx: ctx}
			invocations := 0
			if tc.repair {
				opts.ToolCallRepair = func(_ context.Context, prompt string) (string, error) {
					invocations++
					start := strings.LastIndex(prompt, "<|MISTYPED|broken>")
					out := strings.ReplaceAll(prompt[start:], "MISTYPED", "EPSE")
					out = strings.ReplaceAll(out, "|broken>", "|tool_calls>")
					if tc.change {
						out = strings.ReplaceAll(out, `name="read"`, `name="write"`)
					}
					return out, nil
				}
			}
			turn := BuildTurnFromCollected(sse.CollectResult{Text: tc.raw}, opts)
			status := 200
			if turn.Error != nil {
				status = 422
			}
			recorder.Finish(status, nil, false)
			list := store.List(tooldiagnostics.Query{})
			if list.Total != 1 {
				t.Fatalf("diagnostic missing: %+v", list)
			}
			record, err := store.Get(list.Items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if record.Status != tc.want || record.Pinned != tc.change {
				t.Fatalf("wrong classification: %+v", record)
			}
			if tc.repair && invocations != 1 {
				t.Fatalf("diagnostics triggered extra model calls: %d", invocations)
			}
			if tc.change {
				found := false
				for _, step := range record.Attempts[0].Steps {
					if step.Reason == "repair_integrity_failed" {
						found = true
					}
				}
				if !found {
					t.Fatal("model text success concealed failed integrity check")
				}
			}
		})
	}
}

func TestDiagnosticsDoesNotRecordProseOrIntermediateBuilds(t *testing.T) {
	store := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	r := tooldiagnostics.NewRecorder(store, "chat", false)
	ctx := tooldiagnostics.WithRecorder(context.Background(), r)
	BuildTurnFromCollected(sse.CollectResult{Text: "ordinary answer"}, BuildOptions{DiagnosticContext: ctx})
	r.Finish(200, nil, false)
	BuildTurnFromCollected(sse.CollectResult{Text: `<tool_calls><invoke name="read"></invoke></tool_calls>`}, BuildOptions{})
	if store.List(tooldiagnostics.Query{}).Total != 0 {
		t.Fatal("prose or intermediate build was recorded")
	}
}
