package assistantturn

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/tooldiagnostics"
)

func TestCandidateIsDurableBeforeEOFAndRestartKeepsEvidence(t *testing.T) {
	for _, reasoning := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "diagnostics.json")
		s := tooldiagnostics.New(path)
		ctx := tooldiagnostics.WithRecorder(context.Background(), tooldiagnostics.NewRecorder(s, "test", true))
		progress := DiagnosticProgress{}
		const raw = `<tool_calls><invoke name="read"><parameter name="path"><![CDATA[unfinished`
		text, thinking := raw, ""
		if reasoning {
			text, thinking = "", raw
		}
		progress.Update(ctx, "", text, "", thinking)
		page := s.List(tooldiagnostics.Query{})
		if page.Total != 1 || page.Items[0].Status != "running" {
			t.Fatalf("candidate not durable: %+v", page)
		}
		restarted := tooldiagnostics.New(path)
		record, err := restarted.Get(page.Items[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != "interrupted" || record.Attempts[0].Raw.Text != raw || len(record.Attempts[0].Calls) != 0 {
			t.Fatalf("lost pre-validation evidence: %+v", record)
		}
	}
}

func TestCandidateProbePreservesLiteralContextAcrossChunks(t *testing.T) {
	for _, raw := range []string{"ordinary answer", "Example: `" + `<tool_calls><invoke name="read"></invoke></tool_calls>` + "`", `Example: "<tool_calls>literal</tool_calls>"`} {
		s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
		ctx := tooldiagnostics.WithRecorder(context.Background(), tooldiagnostics.NewRecorder(s, "test", true))
		progress := DiagnosticProgress{}
		for i := 1; i <= len(raw); i++ {
			progress.Update(ctx, "", raw[:i], "", "")
		}
		if page := s.List(tooldiagnostics.Query{}); page.Total != 0 {
			t.Fatalf("literal activated diagnostics: %q %+v", raw, page)
		}
	}
}

func TestLateProgressCannotOverwriteValidatedAttempt(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	ctx := tooldiagnostics.WithRecorder(context.Background(), tooldiagnostics.NewRecorder(s, "test", true))
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: true, Raw: "validated", Calls: []tooldiagnostics.Call{{Name: "read"}}})
	CheckpointToolProgress(ctx, "", tooldiagnostics.Text{Text: `<tool_calls><invoke name="wrong">`, Bytes: 5000, Truncated: true}, tooldiagnostics.Text{})
	record, err := s.Get(s.List(tooldiagnostics.Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempts[0].Raw.Text != "validated" || record.Attempts[0].Calls[0].Name != "read" {
		t.Fatal("late progress replaced validation")
	}
}

func TestBoundedProgressRetainsOriginalByteCount(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	ctx := tooldiagnostics.WithRecorder(context.Background(), tooldiagnostics.NewRecorder(s, "test", true))
	raw := `<tool_calls><invoke name="read">` + strings.Repeat("x", tooldiagnostics.MaxTextBytes)
	CheckpointToolProgress(ctx, "", tooldiagnostics.Text{Text: raw, Bytes: 900000, Truncated: true}, tooldiagnostics.Text{})
	record, err := s.Get(s.List(tooldiagnostics.Query{}).Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempts[0].Raw.Bytes != 900000 || !record.Attempts[0].Raw.Truncated {
		t.Fatal("lost original size")
	}
}

func TestSupersededReasoningCandidateDoesNotTurnOrdinaryAnswerIntoToolRecord(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	r := tooldiagnostics.NewRecorder(s, "test", true)
	ctx := tooldiagnostics.WithRecorder(context.Background(), r)
	progress := DiagnosticProgress{}
	progress.Update(ctx, "", "", "", `<tool_calls><invoke name="read"></invoke></tool_calls>`)
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: false, Raw: "ordinary answer"})
	r.Finish(200, nil, false)
	tooldiagnostics.Candidate(ctx, "reasoning", tooldiagnostics.Capture(`<tool_calls><invoke name="late">`))
	if s.List(tooldiagnostics.Query{}).Total != 0 {
		t.Fatal("superseded reasoning polluted tool diagnostics")
	}
}

func TestFailureKeepsReasoningCandidateEvenWhenBodyContainsProse(t *testing.T) {
	s := tooldiagnostics.New(filepath.Join(t.TempDir(), "diag.json"))
	r := tooldiagnostics.NewRecorder(s, "test", true)
	ctx := tooldiagnostics.WithRecorder(context.Background(), r)
	progress := DiagnosticProgress{}
	progress.Update(ctx, "", "", "", `<tool_calls><invoke name="read">`)
	tooldiagnostics.Observe(ctx, tooldiagnostics.Observation{Relevant: false, Raw: "unfinished answer", ErrorCode: "upstream_interrupted", Error: "interrupted"})
	r.Finish(502, nil, false)
	page := s.List(tooldiagnostics.Query{})
	if page.Total != 1 || !page.Items[0].Pinned || page.Items[0].Status != "failed" {
		t.Fatal("failure evidence discarded")
	}
	record, err := s.Get(page.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempts[0].Source != "reasoning" || record.Attempts[0].Raw.Text != `<tool_calls><invoke name="read">` {
		t.Fatal("failure overwrote the candidate with ordinary prose")
	}
}
