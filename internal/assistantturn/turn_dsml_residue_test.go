package assistantturn

import (
	"strings"
	"testing"

	"ds2api/internal/sse"
)

// TestBuildTurnFromCollectedStripsDSMLResidue is the finalize safety net: a lone
// fullwidth-piped DSML tag left behind by the streaming sieve / repair path is
// removed once from the assembled visible text, without an LLM repair pass.
func TestBuildTurnFromCollectedStripsDSMLResidue(t *testing.T) {
	turn := BuildTurnFromCollected(sse.CollectResult{
		Text: "任务完成。<｜｜DSML｜｜ calls>",
	}, BuildOptions{Model: "deepseek-v4.1-flash", Prompt: "prompt"})
	if strings.Contains(turn.Text, "DSML") {
		t.Fatalf("expected DSML residue stripped, got %q", turn.Text)
	}
	if turn.Text != "任务完成。" {
		t.Fatalf("text mismatch: %q", turn.Text)
	}
}

// TestBuildTurnFromStreamSnapshotStripsDSMLResidue mirrors the net on the
// streaming snapshot path, which feeds the final frame and the archive.
func TestBuildTurnFromStreamSnapshotStripsDSMLResidue(t *testing.T) {
	turn := BuildTurnFromStreamSnapshot(StreamSnapshot{
		RawText:     "回答</｜｜DSML｜｜invoke>",
		VisibleText: "回答</｜｜DSML｜｜invoke>",
	}, BuildOptions{Model: "deepseek-v4.1-flash", Prompt: "prompt"})
	if strings.Contains(turn.Text, "DSML") {
		t.Fatalf("expected DSML residue stripped, got %q", turn.Text)
	}
	if turn.Text != "回答" {
		t.Fatalf("text mismatch: %q", turn.Text)
	}
}
