package usagestats

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestFailedAdminMutationRetainsLedger(t *testing.T) {
	for _, operation := range []string{"import", "clear"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.json")
			store := New(path)
			defer store.Close()
			store.Record("model", "caller", map[string]any{"prompt_tokens": 10}, time.Now())
			before := store.Summary()
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			if operation == "import" {
				_, err = store.Import(before)
			} else {
				err = store.Clear()
			}
			if err == nil {
				t.Fatal("expected disk failure")
			}
			if got := store.Summary(); !reflect.DeepEqual(got, before) {
				t.Fatalf("failed %s changed live totals: before=%+v after=%+v", operation, before, got)
			}
		})
	}
}

func TestInvalidImportDoesNotPartiallyMutateLedger(t *testing.T) {
	for _, bad := range []Entry{
		{PromptTokens: -1, TotalTokens: 10},
		{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 1},
		{CompletionTokens: 1, ReasoningTokens: 2, TotalTokens: 1},
		{TotalTokens: 10, Calls: -1},
		{TotalTokens: 10, Cost: -1},
		{Model: "model\x00other", TotalTokens: 10},
	} {
		store := New(filepath.Join(t.TempDir(), "usage.json"))
		bad.Date = "2026-11-01"
		if bad.Model == "" {
			bad.Model = "model"
		}
		_, err := store.Import([]Entry{{Date: bad.Date, Model: "valid", TotalTokens: 1, Calls: 1}, bad})
		if err == nil || len(store.Summary()) != 0 {
			t.Errorf("invalid import accepted or partially applied: %+v err=%v", bad, err)
		}
		store.Close()
	}
}

func TestImportRejectsMergedCounterOverflow(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	row := Entry{Date: "2026-11-01", Model: "model", TotalTokens: maxUsageCounter, Calls: 1}
	if _, err := store.Import([]Entry{row}); err != nil {
		t.Fatal(err)
	}
	row.TotalTokens = 1
	if _, err := store.Import([]Entry{row}); err == nil {
		t.Fatal("merged counters outside JSON safe integer range were accepted")
	}
	if got := store.Summary(); len(got) != 1 || got[0].Calls != 1 || got[0].TotalTokens != maxUsageCounter {
		t.Fatalf("invalid merge changed the ledger: %+v", got)
	}
}
