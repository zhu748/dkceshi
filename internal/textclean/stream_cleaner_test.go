package textclean

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkStreamCleanerLargeJSON(b *testing.B) {
	for _, size := range []int{16384, 65536} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			raw := `[{"function":{"name":"read","arguments":"` + strings.Repeat(`a\\\"b`, size/6) + `"},"id":"call_x","type":"function"}]`
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				var c StreamCleaner
				for i := 0; i < len(raw); i += 16 {
					if c.Write(raw[i:min(i+16, len(raw))]) != "" {
						b.Fatal("wire leak")
					}
				}
				if c.Flush() != "" {
					b.Fatal("wire tail leak")
				}
			}
		})
	}
}

func TestCleanupPreservesLiteralsAcrossPartitions(t *testing.T) {
	wire := `[{"function":{"name":"read","arguments":"{}"},"id":"call_demo","type":"function"}]`
	for _, literal := range []string{
		"```json\n" + wire + "\n```\n",
		"~~~xml\n<attempt_completion><result>example</result></attempt_completion>\n~~~\n",
		"`" + wire + "`",
		"``example ` <think> text``",
		"<!-- <think> example -->",
		"<![CDATA[<|assistant|>]]>",
		`"example <think>"`,
		"```xml\nunclosed <think>",
	} {
		assertPartitions(t, "before\n"+literal, "before\n"+literal)
	}
	for _, sample := range []struct{ raw, want string }{
		{"before\n" + wire + "\nafter", "before\n\nafter"},
		{`before[{"id":"call_demo","type":"function","function":{"arguments":"{}","name":"read"}}]after`, "beforeafter"},
		{`before< | Tool | >{"content":"result","tool_call_id":"call_demo"}after`, "beforeafter"},
		{"before<think>hidden", "before"},
		{"A<think>B</think>C", "ABC"},
		{"Done.<attempt_completion><result>answer</result></attempt_completion>", "Done.answer"},
		{"Done.answer</result></attempt_completion>", "Done.answer"},
		{"A<|assistant|>B<|end_of_sentence|>C", "ABC"},
		{"```json\n```", "```json\n```"},
		{"<think>İ你好</think>after", "İ你好after"},
		{"<think>outer<think>inner</think>tail</think>after", "outerinnertailafter"},
		{`<think>"literal </think>" hidden`, ""},
		{"<attempt_completion><result>Example: `</attempt_completion>` end</result></attempt_completion>", "Example: `</attempt_completion>` end"},
		{"<think>before\n~~~xml\n</think>\n~~~\nafter</think>end", "before\n~~~xml\n</think>\n~~~\nafterend"},
		{"<think>A<!-- </think> -->B<![CDATA[</think>]]>C</think>end", "A<!-- </think> -->B<![CDATA[</think>]]>Cend"},
	} {
		assertPartitions(t, sample.raw, sample.want)
	}
}

func assertPartitions(t *testing.T, raw, want string) {
	t.Helper()
	if got := CleanVisibleOutput(raw, false); got != want {
		t.Fatalf("batch raw=%q got=%q want=%q", raw, got, want)
	}
	for size := 1; size <= len(raw); size++ {
		var c StreamCleaner
		var out strings.Builder
		for i := 0; i < len(raw); i += size {
			out.WriteString(c.Write(raw[i:min(len(raw), i+size)]))
		}
		out.WriteString(c.Flush())
		if got := out.String(); got != want {
			t.Fatalf("size=%d raw=%q got=%q want=%q", size, raw, got, want)
		}
	}
}

func TestFencedCodeDoesNotWaitForClosingFence(t *testing.T) {
	var c StreamCleaner
	if got := c.Write("```xml\n<tool_calls>example"); got != "```xml\n<tool_calls>example" {
		t.Fatalf("code stalled: %q", got)
	}
	if got := c.Write("\n```\nafter"); got != "\n```\nafter" {
		t.Fatalf("close lost: %q", got)
	}
}
