package gemini

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestQuotedToolExampleKeepsLiteralContextAfterPreview(t *testing.T) {
	raw := `Example: ` + strconv.Quote(`<tool_calls>literal example</tool_calls> [citation:7]`)
	for _, size := range []int{1, 3, 17, len(raw)} {
		w := httptest.NewRecorder()
		rt := newGeminiStreamRuntime(w, http.NewResponseController(w), true, "model", "prompt", false, false, true, []string{"read"}, nil, false, nil)
		for i := 0; i < len(raw); i += size {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
		}
		rt.finalize(false)
		var text strings.Builder
		for _, frame := range extractGeminiSSEFrames(t, w.Body.String()) {
			candidates, _ := frame["candidates"].([]any)
			for _, candidate := range candidates {
				c := candidate.(map[string]any)
				content, _ := c["content"].(map[string]any)
				parts, _ := content["parts"].([]any)
				for _, part := range parts {
					s, _ := part.(map[string]any)["text"].(string)
					text.WriteString(s)
				}
			}
		}
		if text.String() != raw {
			t.Fatalf("size=%d got=%q want=%q", size, text.String(), raw)
		}
	}
}

type brokenStreamResponse struct{ httptest.ResponseRecorder }

func (*brokenStreamResponse) Write([]byte) (int, error) { return 0, errors.New("broken socket") }

func TestFailedOutputStopsBeforeToolRepair(t *testing.T) {
	w := &brokenStreamResponse{*httptest.NewRecorder()}
	rt := newGeminiStreamRuntime(w, http.NewResponseController(w), true, "model", "prompt", false, false, true, []string{"read"}, nil, false, nil)
	rt.toolCallRepair = func(context.Context, string) (string, error) { t.Fatal("repair after output failure"); return "", nil }
	decision := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "ordinary answer"}}})
	if !decision.Stop || !rt.finalize(true) || rt.finalErrorCode != "client_write_error" {
		t.Fatalf("failure ignored: %+v error=%s", decision, rt.finalErrorCode)
	}
}
