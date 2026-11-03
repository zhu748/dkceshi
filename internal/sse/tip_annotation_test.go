package sse

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestTIPAnnotationDoesNotReachCollectedOrStreamedContent(t *testing.T) {
	// Reduced from the Android HAR: the TIP has no op, so snapshot normalization
	// turns it into APPEND before fragment extraction. Node replays this too.
	raw, err := os.ReadFile("../../tests/fixtures/ai_tip.sse")
	if err != nil {
		t.Fatal(err)
	}
	tipAnnotationStream := string(raw)
	for _, thinking := range []bool{false, true} {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(tipAnnotationStream))}
		got := CollectStream(resp, thinking, true)
		if got.ReadError != nil || got.UpstreamError != "" || got.Text != "正文继续" || got.Thinking != "" || got.ToolDetectionThinking != "" {
			t.Fatalf("thinking=%v: unexpected collection: %#v", thinking, got)
		}
		initial := "text"
		if thinking {
			initial = "thinking"
		}
		results, done := StartParsedLinePump(context.Background(), strings.NewReader(tipAnnotationStream), thinking, initial)
		var body strings.Builder
		stopped := false
		for result := range results {
			stopped = stopped || result.Stop
			if result.ErrorMessage != "" || len(result.ToolDetectionThinkingParts) != 0 {
				t.Fatalf("unexpected stream metadata: %#v", result)
			}
			for _, part := range result.Parts {
				if part.Type != "text" {
					t.Fatalf("annotation leaked to another channel: %#v", part)
				}
				body.WriteString(part.Text)
			}
		}
		if err := <-done; err != nil || !stopped || body.String() != "正文继续" {
			t.Fatalf("thinking=%v: streamed %q, stopped=%v, err=%v", thinking, body.String(), stopped, err)
		}
	}
}

func TestTIPAnnotationKeepsAnswerSnapshotHistorySeparate(t *testing.T) {
	raw := `data: {"p":"response/content","v":"正文"}
data: {"p":"response/fragments","v":[{"id":4,"type":"TIP","content":"独立标注"}]}
data: {"v":{"response":{"fragments":[{"id":3,"type":"RESPONSE","content":"正文继续"},{"id":4,"type":"TIP","content":"独立标注"}],"status":"FINISHED"}}}
`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(raw))}
	got := CollectStream(resp, false, true)
	if got.ReadError != nil || got.UpstreamError != "" || got.Text != "正文继续" {
		t.Fatalf("annotation corrupted the answer snapshot: %#v", got)
	}
}
