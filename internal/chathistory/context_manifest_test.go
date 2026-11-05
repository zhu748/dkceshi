package chathistory

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestContextManifestAndExactPromptSurvivePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := New(path)
	const manifest = `{"scope":"normalized_api_context","source_messages":[{"index":0,"bytes":12,"sha256":"digest"}]}`
	raw := json.RawMessage(manifest)
	const prompt = "  exact prompt with prefill \n\t"
	entry, err := store.Start(StartParams{ContextManifest: raw, FinalPrompt: prompt})
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	entry.ContextManifest[0] = '!'
	for _, reader := range []*Store{store, New(path)} {
		got, err := reader.Get(entry.ID)
		if err != nil || string(got.ContextManifest) != manifest || got.FinalPrompt != prompt {
			t.Fatalf("archive changed prompt/manifest: %+v, error=%v", got, err)
		}
	}
}
