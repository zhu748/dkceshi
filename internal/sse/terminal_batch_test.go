package sse

import (
	"strings"
	"testing"
)

func TestTerminalBatchRetainsContentAfterStatus(t *testing.T) {
	result := ParseDeepSeekContentLine([]byte(`data: {"p":"response","o":"APPEND","v":[{"p":"status","v":"FINISHED"},{"p":"content","v":"final answer"}]}`), false, "text")
	var got strings.Builder
	for _, part := range result.Parts {
		got.WriteString(part.Text)
	}
	if !result.Stop || got.String() != "final answer" {
		t.Fatalf("terminal batch dropped content: %+v", result)
	}
}

func TestResponseSnapshotsPreserveContentFilterStatus(t *testing.T) {
	for _, raw := range []string{
		`data: {"v":{"response":{"status":"CONTENT_FILTER","fragments":[{"type":"RESPONSE","content":"blocked"}]}}}`,
		`data: {"p":"response","v":{"quasi_status":"CONTENT_FILTER","fragments":[]}}`,
	} {
		result := ParseDeepSeekContentLine([]byte(raw), false, "text")
		if !result.Stop || !result.ContentFilter || len(result.Parts) != 0 {
			t.Fatalf("snapshot lost filter status: %+v", result)
		}
	}
	// A user-supplied status field in answer text is ordinary content.
	result := ParseDeepSeekContentLine([]byte(`data: {"p":"response/content","v":"{\"status\":\"CONTENT_FILTER\"}"}`), false, "text")
	if result.ContentFilter || result.Stop || len(result.Parts) != 1 {
		t.Fatalf("literal status was treated as metadata: %+v", result)
	}
}
