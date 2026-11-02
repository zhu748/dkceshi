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

func TestQuotedOrdinaryTextStreamsBeforeLiteralCloses(t *testing.T) {
	guard := VisibleGuard{}
	for _, part := range []string{"I will explain \"", "current streaming behavior", "\" then continue."} {
		if got := guard.Write(part); got != part {
			t.Fatalf("delayed/changed quote: got=%q want=%q", got, part)
		}
	}
	if got := guard.Flush(); got != "" {
		t.Fatalf("duplicated quote: %q", got)
	}
}

func TestQuotedUnicodeToolPrefixesWaitForLiteralBoundary(t *testing.T) {
	for _, delimiter := range []string{"<", "＜", "﹤", "〈"} {
		guard := VisibleGuard{}
		markup := delimiter + "|EPSE|tool_calls>literal"
		if got := guard.Write(`"ordinary prefix `); got != `"ordinary prefix ` {
			t.Fatal(got)
		}
		if got := guard.Write(markup); got != "" {
			t.Fatalf("unconfirmed markup previewed: %q", got)
		}
		if got := guard.Write(`"`) + guard.Flush(); got != markup+`"` {
			t.Fatalf("literal changed: %q", got)
		}
	}
}

func TestQuotedPreviewPreservesPendingDisplayContext(t *testing.T) {
	for _, raw := range []string{`before[{"id":"call_demo","type":"function","function":{"name":"read","arguments":"{}"}}]after`, `<think>"literal </think>" hidden`, "Example: `<|EPSE|call name=\"read\">`", `<section data-x="<tag>">text</section>`} {
		want := FilterVisible(raw, "")
		for size := 1; size <= len(raw); size++ {
			guard := VisibleGuard{}
			var got strings.Builder
			for i := 0; i < len(raw); i += size {
				got.WriteString(guard.Write(raw[i:min(i+size, len(raw))]))
			}
			got.WriteString(guard.Flush())
			if got.String() != want {
				t.Fatalf("size=%d got=%q want=%q", size, got.String(), want)
			}
		}
	}
}
