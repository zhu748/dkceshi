package responsehistory

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
)

// Only numeric usage is needed when response history is disabled. Large input
// messages must not be copied, serialized or hashed for an unused archive.
func BenchmarkStartUsageOnlyLargeContext(b *testing.B) {
	usage := usagestats.New(filepath.Join(b.TempDir(), "usage.json"))
	defer usage.Close()
	text := strings.Repeat("long context ", 1<<16)
	params := StartParams{
		Usage: usage, Request: httptest.NewRequest("POST", "/v1/messages", nil),
		Auth: &auth.RequestAuth{CallerID: "caller"}, Surface: "claude.messages",
		Standard: promptcompat.StandardRequest{
			ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: text, HistoryText: text,
			Messages: []any{map[string]any{"role": "user", "content": text}},
			ToolsRaw: []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "description": text}}},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if session := Start(params); session == nil || session.usage == nil {
			b.Fatal("missing usage session")
		}
	}
}
