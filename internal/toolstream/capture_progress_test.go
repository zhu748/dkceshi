package toolstream

import (
	"strings"
	"testing"
)

func TestCaptureProgressOnlyProbesNewStructuralCloses(t *testing.T) {
	var progress captureProgress
	var raw strings.Builder
	raw.WriteString(`<|EPSE|tool_calls><|EPSE|invoke name="write"><|EPSE|parameter name="content"><![CDATA[`)
	if !progress.ready(raw.String()) {
		t.Fatal("first capture must always be parsed")
	}
	for range 2048 {
		raw.WriteString(strings.Repeat("x|", 64))
		if progress.ready(raw.String()) {
			t.Fatal("body-only input scheduled a full parse")
		}
		if progress.offset != raw.Len() {
			t.Fatal("new bytes were not scanned exactly once")
		}
	}
	close := `]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>`
	ready := false
	for _, ch := range close {
		raw.WriteRune(ch)
		ready = progress.ready(raw.String()) || ready
	}
	if !ready {
		t.Fatal("split closing structure never scheduled a parse")
	}
}

func TestLongCaptureKeepsArgumentsAndFollowingCall(t *testing.T) {
	value := strings.Repeat("abc|def\n", 8192)
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + value + `]]></parameter></invoke></tool_calls><tool_calls><invoke name="next"></invoke></tool_calls>`
	var state State
	var events []Event
	for i := 0; i < len(raw); i += 127 {
		events = append(events, ProcessChunk(&state, raw[i:min(i+127, len(raw))], nil)...)
	}
	events = append(events, Flush(&state, nil)...)
	var calls []string
	for _, event := range events {
		if event.Content != "" {
			t.Fatalf("long tool body leaked: %q", event.Content[:min(len(event.Content), 100)])
		}
		for _, call := range event.ToolCalls {
			calls = append(calls, call.Name)
			if call.Name == "write" && call.Input["content"] != value {
				t.Fatal("long argument changed")
			}
		}
	}
	if strings.Join(calls, ",") != "write,next" {
		t.Fatalf("lost/reordered calls: %v", calls)
	}
}

func BenchmarkLongToolCapture(b *testing.B) {
	raw := `<tool_calls><invoke name="write"><parameter name="content"><![CDATA[` + strings.Repeat("x|", 131072) + `]]></parameter></invoke></tool_calls>`
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		var state State
		for i := 0; i < len(raw); i += 128 {
			ProcessChunk(&state, raw[i:min(i+128, len(raw))], nil)
		}
		Flush(&state, nil)
	}
}

func TestUnicodeToolDelimitersDoNotLeakAcrossFragments(t *testing.T) {
	for _, pair := range [][2]string{{"〈", "〉"}, {"﹤", "﹥"}, {"＜", "＞"}} {
		raw := strings.NewReplacer("<", pair[0], ">", pair[1]).Replace(`<|Q7ZK3N|tool_calls><|Q7ZK3N|invoke name="read"><|Q7ZK3N|parameter name="path">a.txt</|Q7ZK3N|parameter></|Q7ZK3N|invoke></|Q7ZK3N|tool_calls>`)
		for _, size := range []int{1, 2, 7, 19} {
			runes := []rune(raw)
			state := State{DeferMalformed: true, Marker: "Q7ZK3M"}
			var events []Event
			for i := 0; i < len(runes); i += size {
				events = append(events, ProcessChunk(&state, string(runes[i:min(i+size, len(runes))]), nil)...)
			}
			events = append(events, Flush(&state, nil)...)
			count := 0
			for _, event := range events {
				if event.Content != "" {
					t.Fatalf("delimiter=%s size=%d leaked=%q", pair[0], size, event.Content)
				}
				count += len(event.ToolCalls)
			}
			if count != 1 {
				t.Fatalf("delimiter=%s size=%d calls=%d", pair[0], size, count)
			}
		}
	}
}
