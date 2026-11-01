package textclean

import "testing"

func TestDisplayMarkersRespectLiteralsAndChunkBoundaries(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"hello CONTENT_FILTERblocked", "hello "},
		{"hello CONTENT_FILTER你好，这个问题我暂时无法回答", "hello "},
		{"CONTENT_FILTER=enabled;", "CONTENT_FILTER=enabled;"},
		{"mention CONTENT_FILTER in docs", "mention CONTENT_FILTER in docs"},
		{"before [citation:7] after", "before  after"},
		{"`[citation:7] CONTENT_FILTERblocked`", "`[citation:7] CONTENT_FILTERblocked`"},
		{"\"[citation:7] CONTENT_FILTERblocked\"", "\"[citation:7] CONTENT_FILTERblocked\""},
		{"```txt\n[citation:7] CONTENT_FILTERblocked\n```", "```txt\n[citation:7] CONTENT_FILTERblocked\n```"},
	} {
		for split := 0; split <= len(tc.raw); split++ {
			c := StreamCleaner{StripReferenceMarkers: true}
			got := c.Write(tc.raw[:split]) + c.Write(tc.raw[split:]) + c.Flush()
			if got != tc.want {
				t.Fatalf("split=%d raw=%q got=%q want=%q", split, tc.raw, got, tc.want)
			}
		}
	}
}
