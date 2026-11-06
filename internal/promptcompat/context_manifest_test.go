package promptcompat

import (
	"strings"
	"testing"
)

func TestContextManifestTracksSourceAcrossExternalization(t *testing.T) {
	r := StandardRequest{Messages: []any{
		map[string]any{"role": "user", "content": "  original\n"},
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c1", "name": "read", "input": map[string]any{"path": "a"}}}},
	}, FinalPrompt: "inline original"}
	before := r.ContextManifest()
	r.SourceContext = r.ContextMessageDigests()
	r.Messages = []any{map[string]any{"role": "user", "content": "current task"}}
	r.FinalPrompt, r.HistoryText, r.CurrentInputFileApplied = "continue", "history original", true
	r.RefFileIDs = []string{"history-file"}
	after := r.ContextManifest()
	if len(after.SourceMessages) != 2 || len(after.LiveMessages) != 1 || !after.Externalized || after.SourceMessages[0] != before.SourceMessages[0] {
		t.Fatalf("source context lost across transport change: %+v", after)
	}
	if after.Prompt.SHA256 == before.Prompt.SHA256 || after.History.Bytes != len(r.HistoryText) || len(after.RefFileIDs) != 1 {
		t.Fatal("final transport inventory is stale")
	}
	r.RefFileIDs[0] = "changed"
	if after.RefFileIDs[0] != "history-file" {
		t.Fatal("manifest aliases request files")
	}
}

func TestContextDigestsDoNotEmbedMessageContent(t *testing.T) {
	r := StandardRequest{Messages: []any{map[string]any{"role": "user", "content": "PRIVATE_CONTENT_SENTINEL"}}}
	m := r.ContextManifest()
	if len(m.SourceMessages) != 1 || len(m.SourceMessages[0].SHA256) != 64 || strings.Contains(m.SourceMessages[0].SHA256, "PRIVATE") {
		t.Fatal("invalid context digest")
	}
}

func TestContextManifestIdentifiesPromptContentAndOmittedMessages(t *testing.T) {
	r := StandardRequest{Messages: []any{
		map[string]any{"role": "user", "content": "  exact\n"},
		map[string]any{"role": "assistant", "content": ""},
		"invalid message",
	}, RawRequest: &ContextDigest{Bytes: 100, SHA256: "raw-body-digest"}}
	m := r.ContextManifest()
	if m.SourceMessages[0].PromptContent != digestContext([]byte("  exact\n")) || m.SourceMessages[1].OmittedReason == "" || m.SourceMessages[2].OmittedReason != "not_a_message_object" {
		t.Fatalf("missing normalization evidence: %+v", m)
	}
	m.RawRequest.SHA256 = "changed"
	if r.RawRequest.SHA256 != "raw-body-digest" {
		t.Fatal("raw digest aliases request")
	}
}
