package sse

import (
	"strings"
	"testing"
)

func BenchmarkParseContentFrame(b *testing.B) {
	for _, tc := range []struct {
		name, line string
		parts      int
	}{
		{"text", `data: {"p":"response/content","v":"Hello from a streaming response"}`, 1},
		{"unicode", `data: {"p":"response/content","v":"流式返回的中文内容与标点。"}`, 1},
		{"batch", `data: {"p":"response","o":"BATCH","v":[` + strings.TrimSuffix(strings.Repeat(`{"p":"content","v":"text"},`, 8), ",") + `]}`, 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			raw := []byte(tc.line)
			b.ReportAllocs()
			for b.Loop() {
				result := ParseDeepSeekContentLine(raw, false, "text")
				if !result.Parsed || len(result.Parts) != tc.parts {
					b.Fatal(result)
				}
			}
		})
	}
}
