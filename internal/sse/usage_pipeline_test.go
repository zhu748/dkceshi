package sse

import (
	"context"
	"ds2api/internal/tokenusage"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUsageSurvivesMetadataOnlyFramesBatchingAndLateMetadata(t *testing.T) {
	raw := "data: {\"v\":{\"response\":{\"message_id\":2,\"accumulated_token_usage\":0}}}\n\n" +
		"data: {\"p\":\"response/content\",\"v\":\"answer\"}\n\n" +
		"data: {\"p\":\"response/output_tokens\",\"v\":\"29\"}\n\n" +
		"data: {\"p\":\"response\",\"o\":\"BATCH\",\"v\":[{\"p\":\"accumulated_token_usage\",\"v\":2551},{\"p\":\"status\",\"v\":\"FINISHED\"}]}\n\n" +
		"data: {\"p\":\"response/input_tokens\",\"v\":11}\n\ndata: [DONE]\n\n"
	collected := CollectStream(&http.Response{Body: io.NopCloser(strings.NewReader(raw))}, false, true)
	if collected.ReadError != nil || collected.Text != "answer" {
		t.Fatalf("collection failed: %+v", collected)
	}
	o := collected.UpstreamUsage.Messages[0]
	if *o.InputTokens != 11 || *o.OutputTokens != 29 || *o.AccumulatedTokens != 2551 {
		t.Fatalf("late metadata lost: %+v", o)
	}
	lines, done := StartParsedLinePump(context.Background(), strings.NewReader(raw), false, "text")
	var observed tokenusage.Snapshot
	var content strings.Builder
	for line := range lines {
		observed.Observe(line.UpstreamUsage)
		for _, p := range line.Parts {
			content.WriteString(p.Text)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	o = observed.Messages[0]
	if content.String() != "answer" || *o.OutputTokens != 29 || *o.AccumulatedTokens != 2551 {
		t.Fatalf("batched stream lost metadata or leaked it as text: %q %+v", content.String(), observed)
	}
}
