package toolstream

import (
	"strings"
	"testing"
)

// collectVisible feeds chunks through the sieve and returns the concatenated
// visible content — the exact text a streaming client would receive as deltas.
func collectVisible(t *testing.T, chunks []string, toolNames []string) string {
	t.Helper()
	var state State
	var textContent string
	for _, chunk := range chunks {
		for _, evt := range ProcessChunk(&state, chunk, toolNames) {
			textContent += evt.Content
		}
	}
	for _, evt := range Flush(&state, toolNames) {
		textContent += evt.Content
	}
	return textContent
}

// TestSieveFlushIncompleteXMLCaptureReleasesDSML reproduces leak path A: the
// model opens a tool_calls wrapper it never closes, then emits a DSML block
// after it. At Flush the never-completed capture is released as visible text
// and the DSML tag leaks to the client with it.
func TestSieveFlushIncompleteXMLCaptureReleasesDSML(t *testing.T) {
	textContent := collectVisible(t, []string{
		"好的，先查一下文件。\n",
		"<tool_calls>\n",
		"<invoke name=\"read_file\">\n",
		"<parameter name=\"path\">README</parameter>\n",
		"<｜｜DSML｜｜ calls>",
	}, []string{"read_file"})
	if strings.Contains(textContent, "DSML") {
		t.Fatalf("DSML tag leaked to visible text via incomplete-capture release: %q", textContent)
	}
}

// TestSieveRejectedXMLBlockReleasesDSML reproduces leak path B: a complete
// tool_calls wrapper whose invokes fail to parse is released as text during
// streaming; a DSML tag inside that rejected block leaks as a delta.
func TestSieveRejectedXMLBlockReleasesDSML(t *testing.T) {
	textContent := collectVisible(t, []string{
		"分析如下。\n",
		"<tool_calls><invoke></invoke></tool_calls>",
		"<｜｜DSML｜｜ calls>",
	}, nil)
	if strings.Contains(textContent, "DSML") {
		t.Fatalf("DSML tag leaked to visible text via rejected-block release: %q", textContent)
	}
}

// TestSieveFlushPartialDSMLTagReleased reproduces leak path D: a DSML tag whose
// closing `>` never arrives is held in pending all stream long and then
// released verbatim at Flush.
func TestSieveFlushPartialDSMLTagReleased(t *testing.T) {
	textContent := collectVisible(t, []string{
		"任务完成。<｜｜DSML｜｜ calls",
	}, nil)
	if strings.Contains(textContent, "DSML") {
		t.Fatalf("partial DSML tag leaked to visible text at flush: %q", textContent)
	}
}

// TestSieveLoneCompleteDSMLTagStaysHidden locks the user-reported shape with no
// tool block around it: a lone complete tag in prose position must be captured
// and discarded, both mid-prose and as the stream's last token.
func TestSieveLoneCompleteDSMLTagStaysHidden(t *testing.T) {
	for name, chunks := range map[string][]string{
		"mid prose":  {"好的，任务完成。<｜｜DSML｜｜ calls>以上是结果。"},
		"last token": {"好的，任务完成。<｜｜DSML｜｜ calls>"},
		"split tag":  {"好的，任务完成。<｜｜DS", "ML｜｜ calls>"},
	} {
		textContent := collectVisible(t, chunks, nil)
		if strings.Contains(textContent, "DSML") {
			t.Fatalf("%s: DSML tag leaked to visible text: %q", name, textContent)
		}
	}
}
