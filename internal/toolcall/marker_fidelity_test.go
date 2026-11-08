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

func TestMarkerNormalizationPreservesJSONMarkdownAndComments(t *testing.T) {
	const literal = "<|Q7ZK3M|invoke>"
	const call = `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="write"><|Q7ZK3M|parameter name="payload">{"snippet":"` + literal + `","open":"<![CDATA[","escaped":"\"` + literal + `"}</|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
	const normalizedCall = `<|EPSE|tool_calls><|EPSE|invoke name="write"><|EPSE|parameter name="payload">{"snippet":"` + literal + `","open":"<![CDATA[","escaped":"\"` + literal + `"}</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, example := range []string{
		"`" + literal + "`", "``" + literal + "``", "```xml\n" + literal + "\n```", "~~~xml\n" + literal + "\n~~~",
		`{"example":"` + literal + `"}`, "<!-- " + literal + " -->", "<![CDATA[" + literal + "]]>",
	} {
		input, want := example+"\n"+call, example+"\n"+normalizedCall
		for split := 0; split <= len(input); split++ {
			n := NewMarkerNormalizer("Q7ZK3M")
			if got := n.Write(input[:split]) + n.Write(input[split:]) + n.Flush(); got != want {
				t.Fatalf("split=%d: literal or real call changed:\n%s", split, got)
			}
		}
		if got := NormalizeToolMarkupMarker(input, "Q7ZK3M"); got != want {
			t.Fatalf("history changed literal data: %s", got)
		}
		if got := ApplyToolMarker(want, "Q7ZK3M"); got != input {
			t.Fatalf("generated history changed literal data: %s", got)
		}
	}
}
