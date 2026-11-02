package responses

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

type brokenStreamResponse struct{ httptest.ResponseRecorder }

func (*brokenStreamResponse) Write([]byte) (int, error) { return 0, errors.New("broken socket") }

func TestFailedOutputStopsBeforeToolRepair(t *testing.T) {
	w := &brokenStreamResponse{*httptest.NewRecorder()}
	rt := newResponsesStreamRuntime(w, http.NewResponseController(w), true, "id", "model", "prompt", false, false, true, []string{"read"}, nil, true, false, promptcompat.DefaultToolChoicePolicy(), "", nil, nil)
	rt.toolCallRepair = func(context.Context, string) (string, error) { t.Fatal("repair after output failure"); return "", nil }
	decision := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "ordinary answer"}}})
	if !decision.Stop || !rt.finalize("stop", true) || rt.finalErrorCode != "client_write_error" {
		t.Fatalf("failure ignored: %+v error=%s", decision, rt.finalErrorCode)
	}
}
