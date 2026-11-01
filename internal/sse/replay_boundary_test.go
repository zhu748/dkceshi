package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func replayWire(t *testing.T, chunks ...map[string]any) string {
	t.Helper()
	var wire strings.Builder
	for _, chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			t.Fatal(err)
		}
		wire.WriteString("data: " + string(data) + "\n\n")
	}
	return wire.String() + "data: [DONE]\n\n"
}

func TestReplayPreservesSourceIdentityAndMixedOperations(t *testing.T) {
	fragment := func(id int, text string) map[string]any {
		return map[string]any{"id": id, "type": "RESPONSE", "content": text}
	}
	for _, fresh := range []string{"APPEND", "SET"} {
		t.Run(fresh, func(t *testing.T) {
			chunks := []map[string]any{
				{"p": "response/fragments", "o": "APPEND", "v": []any{fragment(10, "same"), fragment(11, "same")}},
				{"p": "response", "o": "BATCH", "v": []any{
					map[string]any{"p": "fragments/-1/content", "o": "SET", "v": "same!"},
					map[string]any{"p": "fragments/0/content", "o": "SET", "v": "same"},
					map[string]any{"p": "fragments/-1/content", "o": "APPEND", "v": "!"},
				}},
			}
			if fresh == "APPEND" {
				chunks = append(chunks, map[string]any{"p": "response/fragments", "o": "APPEND", "v": []any{fragment(12, "same")}})
			} else {
				chunks = append(chunks, map[string]any{"p": "response/fragments/2/content", "o": "SET", "v": "same"})
			}
			wire := replayWire(t, chunks...)
			result := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, false, true)
			if result.Text != "samesame!!same" || result.UpstreamError != "" {
				t.Fatalf("lost distinct occurrence: %+v", result)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			lines, done := StartParsedLinePump(ctx, strings.NewReader(wire), false, "text")
			var text strings.Builder
			for line := range lines {
				for _, p := range line.Parts {
					text.WriteString(ContentPartDelta(&text, p))
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if text.String() != result.Text {
				t.Fatalf("stream/collector differ: %q vs %q", text.String(), result.Text)
			}
		})
	}
}

func TestThinkingBoundaryEverySplitAndLiteralEOF(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tag := range []string{"</think>", "</ THINK \n>", "</tHiNk>"} {
			for split := 1; split < len(tag); split++ {
				wire := replayWire(t, map[string]any{"p": "response/thinking_content", "v": "plan" + tag[:split]}, map[string]any{"p": "response/thinking_content", "v": tag[split:] + "answer"})
				got := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, enabled, true)
				if got.Text != "answer" {
					t.Fatalf("enabled=%v tag=%q split=%d: %+v", enabled, tag, split, got)
				}
			}
		}
	}
	for _, suffix := range []string{"<", "</", "</thi", "</think "} {
		wire := replayWire(t, map[string]any{"p": "response/thinking_content", "v": "literal" + suffix})
		got := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, true, true)
		if got.Thinking != "literal"+suffix {
			t.Fatalf("lost EOF literal: %+v", got)
		}
	}
}

func TestThinkingSnapshotDoesNotReplayLiteralState(t *testing.T) {
	prefix := `before <![CDATA[literal `
	full := prefix + `</think>]]> end</think>answer`
	wire := replayWire(t, map[string]any{"p": "response/thinking_content", "o": "APPEND", "v": prefix}, map[string]any{"p": "response/thinking_content", "o": "SET", "v": full})
	got := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, true, true)
	if got.Text != "answer" || got.Thinking != prefix+`</think>]]> end` {
		t.Fatalf("snapshot changed literal context: %+v", got)
	}
}

func TestRewrittenSnapshotFailsClosed(t *testing.T) {
	wire := replayWire(t, map[string]any{"p": "response/content", "v": "old"}, map[string]any{"p": "response/content", "o": "SET", "v": "new"})
	got := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, false, true)
	if got.UpstreamError == "" || got.Text != "old" {
		t.Fatalf("rewritten delivered prefix accepted: %+v", got)
	}
}

func TestExplicitShortReplayAndFragmentProgressStaySeparate(t *testing.T) {
	var text strings.Builder
	text.WriteString("hi")
	if delta := ContentPartDelta(&text, ContentPart{Text: "hi!", Snapshot: true}); delta != "!" {
		t.Fatalf("short replay duplicated: %q", delta)
	}
	wire := replayWire(t,
		map[string]any{"p": "response/content", "v": "body"},
		map[string]any{"p": "response/fragments/-1", "o": "BATCH", "v": []any{map[string]any{"p": "content", "v": "search progress"}}},
		map[string]any{"p": "response/content", "o": "SET", "v": "body!"},
	)
	got := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(wire))}, true, true)
	if got.Text != "body!" || got.UpstreamError != "" {
		t.Fatalf("progress contaminated body snapshot: %+v", got)
	}
}

func BenchmarkThinkingBoundaryMarkup(b *testing.B) {
	for _, size := range []int{4096, 8192, 16384, 32768} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			text := strings.Repeat("<", size)
			b.SetBytes(int64(size))
			for b.Loop() {
				boundary := &ThinkingBoundary{}
				boundary.split(text)
			}
		})
	}
}

func TestMetadataBeforeInitialSnapshotKeepsReplayProvenance(t *testing.T) {
	boundary := &ThinkingBoundary{}
	ParseDeepSeekContentChunk(map[string]any{"v": map[string]any{"response_message_id": 1}}, false, "text", boundary)
	result := ParseDeepSeekContentChunk(map[string]any{"v": map[string]any{"fragments": []any{map[string]any{"type": "RESPONSE", "content": "hi"}, map[string]any{"type": "RESPONSE", "content": "!"}}}}, false, "text", boundary)
	var text strings.Builder
	text.WriteString("hi!")
	for _, part := range result.Parts {
		text.WriteString(ContentPartDelta(&text, part))
	}
	if text.String() != "hi!" {
		t.Fatalf("metadata disabled snapshot dedupe: %q", text.String())
	}
}
