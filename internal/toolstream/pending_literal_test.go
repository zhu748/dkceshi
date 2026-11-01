package toolstream

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestLongHeldLiteralsResumeFiltering(t *testing.T) {
	text := strings.Repeat(`a\"b`, 16384)
	call := `<tool_calls><invoke name="read"><parameter name="path"><![CDATA[secret]]></parameter></invoke></tool_calls>`
	for _, literal := range []string{strconv.Quote(text), "<!--" + text + "-->", "<![CDATA[" + text + "]]>", "<![CDATA[" + text + "]]＞"} {
		for _, size := range []int{1, 17, 4096} {
			guard := VisibleGuard{}
			raw := literal + call + "after"
			var out strings.Builder
			for i := 0; i < len(raw); i += size {
				out.WriteString(guard.Write(raw[i:min(i+size, len(raw))]))
			}
			out.WriteString(guard.Flush())
			if out.String() != literal+"after" {
				t.Fatalf("size=%d literal prefix=%q output length=%d", size, literal[:12], out.Len())
			}
		}
	}
}

func TestHeldLiteralEOFAndInvalidQuotes(t *testing.T) {
	for _, raw := range []string{`"unclosed`, `"escaped\"`, "<!--unclosed", "<![CDATA[unclosed", "\"invalid\nquote"} {
		expected := FilterVisible(raw, "")
		guard := VisibleGuard{}
		var out strings.Builder
		for _, ch := range raw {
			out.WriteString(guard.Write(string(ch)))
		}
		out.WriteString(guard.Flush())
		if out.String() != expected {
			t.Fatalf("raw=%q got=%q want=%q", raw, out.String(), expected)
		}
	}
}

func BenchmarkVisibleGuardQuotedChunks(b *testing.B) {
	for _, n := range []int{4096, 16384, 65536} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			raw := strconv.Quote(strings.Repeat("a", n))
			b.SetBytes(int64(len(raw)))
			for b.Loop() {
				guard := VisibleGuard{}
				for i := 0; i < len(raw); i += 16 {
					guard.Write(raw[i:min(i+16, len(raw))])
				}
				guard.Flush()
			}
		})
	}
}
