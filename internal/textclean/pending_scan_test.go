package textclean

import (
	"strings"
	"testing"
)

func TestLargePendingUnits(t *testing.T) {
	payload := strings.Repeat(`a\"b`, 4096)
	wire := `[{"function":{"name":"read","arguments":"` + payload + `"},"id":"call_demo","type":"function"}]`
	quote := `"` + payload + `<think>"`
	for _, sample := range []struct{ raw, want string }{
		{wire, ""}, {wire[:len(wire)-1], wire[:len(wire)-1]},
		{`[{"example":` + quote + `}]`, `[{"example":` + quote + `}]`},
		{`<|tool|>{"tool_call_id":"call_demo","content":` + quote + `}`, ""},
		{quote, quote}, {"`" + payload + "<think>`", "`" + payload + "<think>`"},
		{"<!--" + payload + "<think>-->", "<!--" + payload + "<think>-->"},
		{"<![CDATA[" + payload + "<think>]]>", "<![CDATA[" + payload + "<think>]]>"},
		{"<think>" + quote + "</think>", quote}, {"<think>" + quote, ""},
	} {
		if got := CleanVisibleOutput(sample.raw, false); got != sample.want {
			t.Fatalf("batch prefix=%q: unexpected cleanup", sample.raw[:40])
		}
		for _, size := range []int{1, 7, 16, 4095, 4096, 4097, 8192, len(sample.raw)} {
			var cleaner StreamCleaner
			var text strings.Builder
			for i := 0; i < len(sample.raw); i += size {
				text.WriteString(cleaner.Write(sample.raw[i:min(i+size, len(sample.raw))]))
			}
			text.WriteString(cleaner.Flush())
			if text.String() != sample.want {
				t.Fatalf("size=%d prefix=%q: unexpected cleanup", size, sample.raw[:40])
			}
		}
	}
}
