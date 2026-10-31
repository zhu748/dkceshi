package sse

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPromptEchoCollectionKeepsReasoningSeparate(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	var wire strings.Builder
	for _, part := range []map[string]any{
		{"p": "response/thinking_content", "v": "I will edit the test."},
		{"p": "response/content", "v": prefix},
	} {
		data, err := json.Marshal(part)
		if err != nil {
			t.Fatal(err)
		}
		wire.WriteString("data: " + string(data) + "\n\n")
	}
	wire.WriteString("data: [DONE]\n\n")
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire.String()))}
	result := CollectStream(resp, true, true, prefix)
	if result.Text != "" || result.Thinking != "I will edit the test." || result.Cut != CutPromptEcho || result.ReadError != nil {
		t.Fatalf("channel separation or echo classification failed: %+v", result)
	}
}
