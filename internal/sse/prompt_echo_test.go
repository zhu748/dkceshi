package sse

import (
	"strings"
	"testing"
)

func TestPromptEchoPolicyAcrossEveryChunkSize(t *testing.T) {
	for _, first := range []string{"<｜System｜>custom instruction", "Serialized conversation. Each message ends with "} {
		prompt := "<｜System｜>custom instruction\n\nSerialized conversation. Each message ends with [EndMessage:0123456789abcdef]."
		input := "\n\n" + first + "\nnever deliver this"
		for size := 1; size <= len(input); size++ {
			policy := ContentPolicy{Prompt: prompt}
			var out strings.Builder
			for i := 0; i < len(input); i += size {
				out.WriteString(policy.Feed(input[i:min(i+size, len(input))]))
			}
			out.WriteString(policy.Flush())
			if out.Len() != 0 || policy.Cut() != CutPromptEcho {
				t.Fatalf("size=%d leaked %q cut=%s", size, out.String(), policy.Cut())
			}
		}
	}
}

func TestPromptEchoPolicyPreservesLiteralsAndEOFTails(t *testing.T) {
	prompt := "<｜System｜>custom instruction\n\nSerialized conversation. Each message ends with [EndMessage:0123456789abcdef]."
	for _, input := range []string{"answer", "<｜System｜>different instruction", "<｜System", "Serialized conversation.", "\n\n", "`<｜System｜>custom instruction`", "```text\n<｜System｜>custom instruction\n```", "I was asked to explain <｜System｜>custom instruction", strings.Repeat(" ", 1024)} {
		for size := 1; size <= len(input); size++ {
			policy := ContentPolicy{Prompt: prompt}
			var out strings.Builder
			for i := 0; i < len(input); i += size {
				out.WriteString(policy.Feed(input[i:min(i+size, len(input))]))
			}
			out.WriteString(policy.Flush())
			out.WriteString(policy.Flush())
			if out.String() != input || policy.Cut() != CutNone {
				t.Fatalf("size=%d input=%q output=%q cut=%s", size, input, out.String(), policy.Cut())
			}
		}
	}
}
