package sse

import "testing"

const testTranscriptBoundary = "[EndMessage:3237101fbf162c60]"

func TestTranscriptBoundaryCutsAcrossEveryChunkSize(t *testing.T) {
	input := "done\n" + testTranscriptBoundary + "\n[Assistant]:invented"
	for size := 1; size <= len(input); size++ {
		var policy ContentPolicy
		out := ""
		for i := 0; i < len(input); i += size {
			out += policy.Feed(input[i:min(i+size, len(input))])
		}
		if out != "done\n" || policy.Cut() != CutTranscriptBoundary {
			t.Fatalf("size=%d output=%q cut=%q", size, out, policy.Cut())
		}
	}
}

func TestTranscriptBoundaryPreservesLiterals(t *testing.T) {
	for _, literal := range []string{
		"<![CDATA[" + testTranscriptBoundary + "]]>",
		`"` + testTranscriptBoundary + `"`,
		"`" + testTranscriptBoundary + "`",
		"```txt\n" + testTranscriptBoundary + "\n```\n",
		"~~~txt\n" + testTranscriptBoundary + "\n~~~\n",
	} {
		input := literal + "\n" + testTranscriptBoundary + "\ninvented"
		for size := 1; size <= len(input); size++ {
			var policy ContentPolicy
			out := ""
			for i := 0; i < len(input); i += size {
				out += policy.Feed(input[i:min(i+size, len(input))])
			}
			if out != literal+"\n" || policy.Cut() != CutTranscriptBoundary {
				t.Fatalf("size=%d literal=%q output=%q cut=%q", size, literal, out, policy.Cut())
			}
		}
	}
}

func TestTranscriptBoundaryRejectsOrdinaryBracketedText(t *testing.T) {
	for _, text := range []string{
		"[EndMessage:short]", "[EndMessage:3237101fbf162c6g]",
		"[EndMessage:3237101fbf162c600]", "[EndMessage is documentation]",
	} {
		input := text + "\nend"
		for size := 1; size <= len(input); size++ {
			var policy ContentPolicy
			out := ""
			for i := 0; i < len(input); i += size {
				out += policy.Feed(input[i:min(i+size, len(input))])
			}
			if out != input || policy.Cut() != CutNone {
				t.Fatalf("size=%d input=%q output=%q cut=%q", size, input, out, policy.Cut())
			}
		}
	}
}
