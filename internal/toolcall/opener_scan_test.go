package toolcall

import (
	"fmt"
	"strings"
	"testing"
)

func TestProtocolPrefixDelimiterRunsRespectLiterals(t *testing.T) {
	for _, prefix := range []string{strings.Repeat("<", 512), strings.Repeat("＜<", 256)} {
		for _, literal := range []string{`![CDATA[<|EPSE|invoke name="literal">]]>`, `!-- <|EPSE|invoke name="literal"> -->`} {
			text := prefix + literal + ` <|EPSE|invoke name="real">`
			want := strings.LastIndex(text, "<|EPSE|")
			if index, _ := FindToolProtocolPrefixOutsideIgnored(text, 0); index != want {
				t.Fatalf("literal became a candidate: index=%d want=%d", index, want)
			}
			if tag, ok := FindToolMarkupTagOutsideIgnored(text, 0); !ok || tag.Start != want {
				t.Fatalf("literal became markup: %+v", tag)
			}
		}
		if index, _ := FindToolProtocolPrefixOutsideIgnored(prefix+">", 0); index != -1 {
			t.Fatal("ordinary delimiter run became protocol")
		}
		if index, _ := FindToolProtocolPrefixOutsideIgnored(prefix+`|EPSE|invoke name="real">`, 0); index != 0 {
			t.Fatal("repeated opening delimiters lost earliest source boundary")
		}
	}
}

func BenchmarkProtocolPrefixDelimiterRun(b *testing.B) {
	for _, size := range []int{8192, 131072} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			text := strings.Repeat("<", size) + ">"
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				FindToolProtocolPrefixOutsideIgnored(text, 0)
			}
		})
	}
}

func BenchmarkMalformedDelimiterParsing(b *testing.B) {
	text := strings.Repeat("<", 8192) + ">"
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseToolCallsDetailed(text, nil)
	}
}
