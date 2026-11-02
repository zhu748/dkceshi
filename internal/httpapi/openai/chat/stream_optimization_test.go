package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

type brokenStreamResponse struct{ httptest.ResponseRecorder }

func (*brokenStreamResponse) Write([]byte) (int, error) { return 0, errors.New("broken connection") }

func TestFailedOutputStopsBeforeToolRepair(t *testing.T) {
	w := &brokenStreamResponse{*httptest.NewRecorder()}
	rt := newChatStreamRuntime(w, http.NewResponseController(w), true, "id", 1, "model", "prompt", false, false, true, nil, nil, promptcompat.DefaultToolChoicePolicy(), true, false)
	rt.toolCallRepair = func(context.Context, string) (string, error) { t.Fatal("repair after socket failure"); return "", nil }
	decision := rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: "already generated answer"}}})
	if !decision.Stop || !rt.finalize("stop", true) || rt.finalErrorCode != "client_write_error" || rt.visibleTextEmitted {
		t.Fatalf("output failure lost: decision=%+v runtime=%+v", decision, rt)
	}
}

func TestPrepareReplyFailureReclaimsLease(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		h := &Handler{}
		lease := h.holdStreamLease(&auth.RequestAuth{}, promptcompat.StandardRequest{}, "session")
		r := httptest.NewRequest("POST", "/", nil)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		if canceled {
			cancel()
		}
		r = r.WithContext(ctx)
		w := &brokenStreamResponse{*httptest.NewRecorder()}
		if h.writePreparedLease(w, r, lease, map[string]any{"lease_id": lease}) {
			t.Fatal("failed prepare accepted")
		}
		if _, ok := h.lookupStreamLease(lease); ok {
			t.Fatal("failed prepare leaked lease")
		}
	}
}

func TestVercelHistoryUsesOriginalClockAndError(t *testing.T) {
	store := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	h := &Handler{ChatHistory: store}
	started := time.Now().Add(-time.Minute)
	h.recordVercelStreamCompletion(httptest.NewRequest("POST", "/", nil), &auth.RequestAuth{CallerID: "caller"}, promptcompat.StandardRequest{ResponseModel: "model"}, map[string]any{"error": "missing path", "error_status": 422, "error_code": "invalid_tool_arguments"}, started)
	snapshot, err := store.Snapshot()
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	entry := snapshot.Items[0]
	if entry.ElapsedMs < 60000 || entry.CreatedAt != started.UnixMilli() || entry.StatusCode != 422 || entry.FinishReason != "invalid_tool_arguments" {
		t.Fatalf("lost completion metadata: %+v", entry)
	}
}
