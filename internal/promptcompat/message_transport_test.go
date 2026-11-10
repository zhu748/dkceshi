package promptcompat

import (
	"encoding/json"
	"testing"
)

func TestMessageTransportCoverageChecksOccurrencesAndExtensions(t *testing.T) {
	first := map[string]any{"role": "user", "content": "SAME", "metadata": map[string]any{"n": json.Number("9007199254740993")}}
	second := map[string]any{"role": "assistant", "content": "SAME", "prefix": true}
	source := []any{first, first, second}
	if err := ValidateMessageTransportCoverage(source, []any{first}, []any{first, second}); err != nil {
		t.Fatal(err)
	}
	for _, live := range [][]any{{second}, {first, map[string]any{"role": "assistant", "content": "SAME"}}, {map[string]any{"role": "user", "content": "SAME"}, second}} {
		if ValidateMessageTransportCoverage(source, []any{first}, live) == nil {
			t.Fatal("missing occurrence or metadata passed coverage")
		}
	}
}

func TestManifestRoutesEveryMessageOccurrenceOrExplainsItsAbsence(t *testing.T) {
	first := map[string]any{"role": "user", "content": "REPEATED"}
	r := StandardRequest{Messages: []any{first, first, map[string]any{"role": "assistant", "content": "PREFILL", "prefix": true}}}
	r.SourceContext = r.ContextMessageDigests()
	r.HistoryContext = (StandardRequest{Messages: []any{first}}).ContextMessageDigests()
	r.Messages = r.Messages[1:]
	manifest := r.ContextManifest()
	routes := manifest.MessageTransports
	if len(routes) != 3 || routes[0].HistoryIndex == nil || routes[1].LiveIndex == nil || routes[1].OmittedReason != "" || routes[2].LiveIndex == nil {
		t.Fatalf("occurrence matching changed: %+v", routes)
	}
	r.HistoryContext = nil
	if r.ContextManifest().MessageTransports[1].OmittedReason != "message_has_no_transport_destination" {
		t.Fatal("missing duplicate message was not explained")
	}
}
