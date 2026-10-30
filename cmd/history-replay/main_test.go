package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ds2api/internal/chathistory"
)

func TestArchiveReplayRecoversNativeCallsAndUsesRecordedSchemas(t *testing.T) {
	content := `<｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="lookup"><｜｜DSML｜｜ parameter name="id" string="false">9007199254740993</｜｜DSML｜｜ parameter></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls>`
	path := filepath.Join(t.TempDir(), "history.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entryWriter, err := writer.Create("chat_history.json.d/chat_test.json")
	if err != nil {
		t.Fatal(err)
	}
	entry := chathistory.Entry{ID: "chat_test", Surface: "openai.chat_completions", Content: content, Usage: map[string]any{"prompt_tokens": json.Number("123")}}
	if err := json.NewEncoder(entryWriter).Encode(struct{ Item chathistory.Entry }{entry}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := loadArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	report, err := audit(entries)
	if err != nil {
		t.Fatal(err)
	}
	if report.MainToolCalls != 1 || report.RecoveredRequests != 1 || report.Entries[0].PromptTokens != 123 || report.Entries[0].ErrorCode != "" || report.Entries[0].HasToolSchemas {
		t.Fatalf("unexpected report: %+v", report)
	}
	// A known schema makes missing required fields an error, without guessing
	// values or invoking an upstream repair service.
	entries[0].Tools = json.RawMessage(`[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","required":["path"]}}}]`)
	report, err = audit(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Entries[0].HasToolSchemas || report.Entries[0].ErrorCode != "invalid_tool_arguments" {
		t.Fatalf("missing argument accepted: %+v", report)
	}
}
