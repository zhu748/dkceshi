package toolcall

import (
	"strings"
	"testing"
)

func TestMarkerNormalizationPreservesLiteralArgumentsAtEveryBoundary(t *testing.T) {
	const marker = "Q7ZK3M"
	for _, delimiters := range [][2]string{{"<![CDATA[", "]]>"}, {"＜！［ＣＤＡＴＡ［", "］］＞"}} {
		literal := "C:/Q7ZK3M/file.txt <|Q7ZK3M|invoke> 中文"
		input := `Q7ZK3M <|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="content">` + delimiters[0] + literal + delimiters[1] + `</|Q7ZK3M|parameter></|Q7ZK3M|invoke> Q7ZK3M`
		want := `Q7ZK3M <|EPSE|invoke name="write"><|EPSE|parameter name="content">` + delimiters[0] + literal + delimiters[1] + `</|EPSE|parameter></|EPSE|invoke> Q7ZK3M`
		for size := 1; size <= len(input); size++ {
			n := NewMarkerNormalizer(marker)
			var out strings.Builder
			for i := 0; i < len(input); i += size {
				out.WriteString(n.Write(input[i:min(i+size, len(input))]))
			}
			out.WriteString(n.Flush())
			if out.String() != want {
				t.Fatalf("chunk=%d: literal changed: %q", size, out.String())
			}
		}
	}
}

func TestMarkerNormalizationDoesNotRewriteLongerWords(t *testing.T) {
	input := "value Q7ZK3M <|Q7ZK3Mextra> <|Q7ZK3Mｅｘｔｒａ> tail"
	for size := 1; size <= len(input); size++ {
		n := NewMarkerNormalizer("Q7ZK3M")
		var out strings.Builder
		for i := 0; i < len(input); i += size {
			out.WriteString(n.Write(input[i:min(i+size, len(input))]))
		}
		out.WriteString(n.Flush())
		if out.String() != input {
			t.Fatalf("chunk=%d: ordinary word rewritten: %q", size, out.String())
		}
	}
}
