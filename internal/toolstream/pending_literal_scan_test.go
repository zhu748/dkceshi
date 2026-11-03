package toolstream

import (
	"fmt"
	"strings"
	"testing"
)

func TestPendingLiteralSplitClosers(t *testing.T) {
	for _, tc := range []struct{ open, close string }{
		{"<!--", "-->"}, {"<![CDATA[", "]]>"},
		{"<![CDATA[", "]]＞"}, {"<![CDATA[", "]]﹥"}, {"<![CDATA[", "]]〉"},
	} {
		for split := 0; split <= len(tc.close); split++ {
			t.Run(fmt.Sprintf("%s/%d", tc.close, split), func(t *testing.T) {
				p := newPendingLiteral(tc.open + "body")
				if p == nil || p.feed("unrelated ]-＞ bytes") {
					t.Fatal("ordinary body closed the literal")
				}
				first := p.feed(tc.close[:split])
				if first != (split == len(tc.close)) {
					t.Fatal("incorrect boundary detection in first chunk")
				}
				if !first && !p.feed(tc.close[split:]+"after") {
					t.Fatal("split closer was missed")
				}
			})
		}
	}
}

func TestPendingLiteralLongBodyAndFalsePrefixes(t *testing.T) {
	for _, open := range []string{"<!--", "<![CDATA["} {
		p := newPendingLiteral(open)
		for _, chunk := range []string{strings.Repeat("x", 65536), "]", "-", "]", "＞", "--", "x>", "]]", "x>", ""} {
			if p.feed(chunk) {
				t.Fatalf("%s closed on %q", open, chunk[:min(20, len(chunk))])
			}
		}
		close := "-->"
		if open == "<![CDATA[" {
			close = "]]>"
		}
		for i := range len(close) - 1 {
			if p.feed(close[i : i+1]) {
				t.Fatal("closed before final byte")
			}
		}
		if !p.feed(close[len(close)-1:]) {
			t.Fatal("closer lost after false prefixes")
		}
	}
}

func BenchmarkPendingLiteralDelimited(b *testing.B) {
	for _, open := range []string{"<!--", "<![CDATA["} {
		for _, size := range []int{16, 128, 4096} {
			b.Run(fmt.Sprintf("%s/chunk-%d", open, size), func(b *testing.B) {
				body := strings.Repeat("plain body ", 6000)
				b.SetBytes(int64(len(body)))
				b.ReportAllocs()
				for b.Loop() {
					p := newPendingLiteral(open)
					for i := 0; i < len(body); i += size {
						if p.feed(body[i:min(i+size, len(body))]) {
							b.Fatal("body closed literal")
						}
					}
				}
			})
		}
	}
}
