package toolcall

import (
	"strings"
	"testing"
)

// TestStripDSMLResidueLoneFullwidthTag covers the user-reported leak shape: a
// lone fullwidth-piped DSML tag with a space before the local name surviving in
// an otherwise clean reply.
func TestStripDSMLResidueLoneFullwidthTag(t *testing.T) {
	text := `好的，任务完成。<｜｜DSML｜｜ calls>`
	got := StripDSMLResidue(text)
	if strings.Contains(got, "DSML") {
		t.Fatalf("expected lone DSML tag removed, got %q", got)
	}
	if !strings.Contains(got, "好的，任务完成。") {
		t.Fatalf("expected prose preserved, got %q", got)
	}
}

// TestStripDSMLResidueFullWrapper covers the paired-wrapper shape with the same
// fullwidth-piped separator style.
func TestStripDSMLResidueFullWrapper(t *testing.T) {
	text := "before <｜｜DSML｜｜invoke name=\"read\"></｜｜DSML｜｜invoke> after"
	got := StripDSMLResidue(text)
	if strings.Contains(got, "DSML") {
		t.Fatalf("expected DSML wrapper removed, got %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected prose preserved, got %q", got)
	}
}

// TestStripDSMLResidueOrphanClosingTag covers an unmatched closing shell.
func TestStripDSMLResidueOrphanClosingTag(t *testing.T) {
	text := `result ready </｜｜DSML｜｜invoke> done`
	got := StripDSMLResidue(text)
	if strings.Contains(got, "DSML") {
		t.Fatalf("expected orphan DSML closing tag removed, got %q", got)
	}
	if !strings.Contains(got, "result ready") || !strings.Contains(got, "done") {
		t.Fatalf("expected prose preserved, got %q", got)
	}
}

// TestStripDSMLResidueUnclosedTagKeepsTrailingProse covers the truncated tail:
// the unclosed tag run goes, the prose after it stays.
func TestStripDSMLResidueUnclosedTagKeepsTrailingProse(t *testing.T) {
	text := "answer <DSML calls><DSML invoke name=\"read\"> and the explanation"
	got := StripDSMLResidue(text)
	if strings.Contains(got, "DSML") {
		t.Fatalf("expected unclosed DSML tags removed, got %q", got)
	}
	if !strings.Contains(got, "answer") || !strings.Contains(got, "and the explanation") {
		t.Fatalf("expected prose preserved, got %q", got)
	}
}

// TestStripDSMLResidueKeepsFencedDocs ensures a documented DSML example inside
// a fenced code block is documentation, not residue.
func TestStripDSMLResidueKeepsFencedDocs(t *testing.T) {
	text := "example:\n```\n<｜｜DSML｜｜ calls>\n```\nbye"
	got := StripDSMLResidue(text)
	if !strings.Contains(got, "<｜｜DSML｜｜ calls>") {
		t.Fatalf("expected fenced DSML example preserved, got %q", got)
	}
}

// TestStripDSMLResidueCleanTextUnchanged ensures the net is a no-op on clean
// text (including surrounding whitespace, which must not be trimmed).
func TestStripDSMLResidueCleanTextUnchanged(t *testing.T) {
	cases := []string{
		"",
		"plain reply",
		"  padded plain reply  ",
		"inline `<｜｜DSML｜｜ calls>` code demo",
		"<|EPSE|tool_calls><|EPSE|invoke name=\"Bash\"><|EPSE|parameter name=\"command\">pwd</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>",
	}
	for _, text := range cases {
		if got := StripDSMLResidue(text); got != text {
			t.Fatalf("expected clean text unchanged, got %q for input %q", got, text)
		}
	}
}

// TestStripDSMLResidueTrailingPartialTag covers the unterminated tail cut: a
// fragment whose `>` never arrived (stream ended) is removed, with and without
// a complete residue preceding it.
func TestStripDSMLResidueTrailingPartialTag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"任务完成。<｜｜DSML｜｜ calls", "任务完成。"},
		{"done <｜｜DS", "done"},
		{"keep <｜｜DSML｜｜ calls></｜｜DSML｜｜ calls><｜｜DSML｜｜ cal", "keep"},
	}
	for _, c := range cases {
		if got := StripDSMLResidue(c.in); got != c.want {
			t.Fatalf("StripDSMLResidue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStripDSMLResidueKeepsInnocentTrailingLessThan ensures prose ending in a
// bare `<` or a non-DSML fragment is never cut by the tail handling.
func TestStripDSMLResidueKeepsInnocentTrailingLessThan(t *testing.T) {
	cases := []string{
		"a < b 的解释",
		"以 < 结尾",
		"看 <tool",
	}
	for _, text := range cases {
		if got := StripDSMLResidue(text); got != text {
			t.Fatalf("expected innocent tail preserved, got %q for input %q", got, text)
		}
	}
}

// TestStripDSMLResidueIdempotent ensures running the net twice is stable.
func TestStripDSMLResidueIdempotent(t *testing.T) {
	text := "keep <｜｜DSML｜｜ calls> me"
	once := StripDSMLResidue(text)
	twice := StripDSMLResidue(once)
	if once != twice {
		t.Fatalf("expected idempotent strip, once=%q twice=%q", once, twice)
	}
}
