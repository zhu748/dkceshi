package chat

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/tokenusage"
)

type captureUsageDS struct{ inlineUploadDSStub }
type captureUsageResolver struct{}

func (*captureUsageDS) NewUsageResolver(*auth.RequestAuth, string) tokenusage.Resolver {
	return captureUsageResolver{}
}
func (captureUsageResolver) ResolveUpstreamUsage(context.Context, int) (tokenusage.Observation, error) {
	input, output, total := 58, 23, 81
	return tokenusage.Observation{MessageID: 2, InputTokens: &input, OutputTokens: &output, TotalTokens: &total}, nil
}

func TestVercelPreviewAndFinalizationUsePairedUpstreamCounters(t *testing.T) {
	h := &Handler{DS: &captureUsageDS{}}
	id := h.holdStreamLease(&auth.RequestAuth{CallerID: "caller"}, promptcompat.StandardRequest{ResponseModel: "model", PromptTokenText: "prompt", RefFileTokens: 86700}, "session")
	req := map[string]any{"usage_accounting_version": 1, "finish_reason": "stop", "content": "answer", "upstream_usage": map[string]any{"messages": []any{map[string]any{"message_id": 2, "accumulated_tokens": 81}}}}
	rec := httptest.NewRecorder()
	h.previewStreamUsage(rec, httptest.NewRequest("POST", "/", nil), id, req)
	var reply map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatal(err)
	}
	usage := reply["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(58) || usage["completion_tokens"] != float64(23) || usage["total_tokens"] != float64(81) {
		t.Fatalf("preview estimated known counts or added files twice: %s", rec.Body)
	}
	lease, ok := h.releaseStreamLease(id)
	if !ok {
		t.Fatal("missing lease")
	}
	final := h.finalizeLeaseUsage(context.Background(), lease, req)
	if final["prompt_tokens"] != 58 || final["completion_tokens"] != 23 || final["total_tokens"] != 81 {
		t.Fatalf("finalization changed preview: %+v", final)
	}
	if final["ds2api"].(*tokenusage.Metadata).Source != "upstream" {
		t.Fatal("missing upstream provenance")
	}
}
