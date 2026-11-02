package claude

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ds2api/internal/sse"
)

func TestQuotedToolExampleKeepsLiteralContextAfterPreview(t *testing.T) {
	raw := `Example: ` + strconv.Quote(`<tool_calls>literal example</tool_calls> [citation:7]`)
	for _, size := range []int{1, 3, 17, len(raw)} {
		w := httptest.NewRecorder()
		rt := newClaudeStreamRuntime(w, http.NewResponseController(w), true, "model", nil, false, false, true, []string{"read"}, nil, "prompt", nil)
		for i := 0; i < len(raw); i += size {
			rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
		}
		rt.finalize("end_turn", false)
		if got := collectClaudeTextDeltas(parseClaudeFrames(t, w.Body.String())); got != raw {
			t.Fatalf("size=%d got=%q want=%q", size, got, raw)
		}
	}
}

type brokenStreamResponse struct{ httptest.ResponseRecorder }

func (*brokenStreamResponse) Write([]byte) (int, error) { return 0, errors.New("broken socket") }

func TestFailedOutputStopsBeforeToolRepair(t *testing.T) {
	w := &brokenStreamResponse{*httptest.NewRecorder()}
	rt := newClaudeStreamRuntime(w, http.NewResponseController(w), true, "model", nil, false, false, true, []string{"read"}, nil, "prompt", nil)
	rt.toolCallRepair = func(context.Context, string) (string, error) { t.Fatal("repair after output failure"); return "", nil }
	decision := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "ordinary answer"}}})
	if !decision.Stop || !rt.finalize("end_turn", true) || rt.finalErrorCode != "client_write_error" {
		t.Fatalf("failure ignored: %+v error=%s", decision, rt.finalErrorCode)
	}
}
