package toolstream

import (
	"strconv"
	"strings"
	"testing"
)

func TestVisibleGuardAcrossAllSplits(t *testing.T) {
	call := `<|EPSE|tool_calls><|EPSE|invoke name="run"><|EPSE|parameter name="cmd"><![CDATA[secret]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	for _, tc := range []struct{ raw, want string }{
		{"before " + call + " after", "before  after"},
		{"before <|EPSE|tool_calls>", "before "},
		{"before <|epse|invoke name=\"run\"", "before "},
		{"before <｜ＥＰＳＥ｜call name=\"run\">secret</｜ＥＰＳＥ｜call>", "before "},
		{"before <｜EPSE｜tool_ca", "before "},
		{"before ＜｜DSML｜ calls", "before "},
		{"before <invoke name=\"run\"", "before "},
		{"1 < 2 and x > y; a trailing <", "1 < 2 and x > y; a trailing <"},
		{"`" + call + "`", "`" + call + "`"},
		{"```xml\n" + call + "\n```", "```xml\n" + call + "\n```"},
		{"~~~xml\n" + call + "\n~~~", "~~~xml\n" + call + "\n~~~"},
		{`{"example":` + strconv.Quote(call) + `}`, `{"example":` + strconv.Quote(call) + `}`},
		{`<![CDATA[<|EPSE|tool_calls>]]>`, `<![CDATA[<|EPSE|tool_calls>]]>`},
		{`<!-- <|EPSE|tool_calls> -->`, `<!-- <|EPSE|tool_calls> -->`},
	} {
		runes := []rune(tc.raw)
		for split := 0; split <= len(runes); split++ {
			guard := VisibleGuard{}
			got := guard.Write(string(runes[:split])) + guard.Write(string(runes[split:])) + guard.Flush()
			if got != tc.want {
				t.Fatalf("split=%d raw=%q got=%q want=%q", split, tc.raw, got, tc.want)
			}
		}
		guard := VisibleGuard{}
		var got strings.Builder
		for _, ch := range runes {
			got.WriteString(guard.Write(string(ch)))
		}
		got.WriteString(guard.Flush())
		if got.String() != tc.want {
			t.Fatalf("rune chunks raw=%q got=%q want=%q", tc.raw, got.String(), tc.want)
		}
	}
}

func TestVisibleGuardCallerMarkerAndOrdinaryThinking(t *testing.T) {
	guard := VisibleGuard{Marker: "Q7ZK3M"}
	raw := `think <|Q7ZK3M|invoke name="run"><|Q7ZK3M|parameter name="cmd">secret</|Q7ZK3M|parameter></|Q7ZK3M|invoke> done`
	var got strings.Builder
	for _, ch := range raw {
		got.WriteString(guard.Write(string(ch)))
	}
	got.WriteString(guard.Flush())
	if got.String() != "think  done" {
		t.Fatalf("caller marker leaked: %q", got.String())
	}
	if got := FilterVisible("normal reasoning", ""); got != "normal reasoning" {
		t.Fatalf("ordinary reasoning changed: %q", got)
	}
}

func TestDeferredBodySieveKeepsSplitLiteralsBeforeRealCall(t *testing.T) {
	call := `<tool_calls><invoke name="run"></invoke></tool_calls>`
	for _, literal := range []string{strconv.Quote(call), "<![CDATA[" + call + "]]>", "<!-- " + call + " -->", "`" + call + "`"} {
		raw := literal + "\n" + call
		runes := []rune(raw)
		for split := 0; split <= len(runes); split++ {
			state := State{DeferMalformed: true}
			events := ProcessChunk(&state, string(runes[:split]), nil)
			events = append(events, ProcessChunk(&state, string(runes[split:]), nil)...)
			events = append(events, Flush(&state, nil)...)
			var visible strings.Builder
			calls := 0
			for _, event := range events {
				visible.WriteString(event.Content)
				calls += len(event.ToolCalls)
			}
			if visible.String() != literal+"\n" || calls != 1 {
				t.Fatalf("split=%d literal=%q visible=%q calls=%d", split, literal, visible.String(), calls)
			}
		}
	}
}
