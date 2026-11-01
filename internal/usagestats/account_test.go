package usagestats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func TestAccountLedgerSurvivesImportAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	store := New(path)
	defer store.Close()
	at := time.Date(2026, 11, 1, 23, 59, 0, 0, gmt8)
	usage := map[string]any{"prompt_tokens": 10, "completion_tokens": 5}
	store.RecordAccount("model", "caller", "account:a", "A", usage, at)
	store.RecordAccount("model", "caller", "account:b", "B", usage, at)
	store.RecordAccount("model", "caller", "account:a", "older name", usage, at.Add(-time.Hour))
	entries := store.Summary()
	if len(entries) != 2 || entries[0].AccountID != "account:a" || entries[0].Calls != 2 || entries[0].AccountLabel != "A" {
		t.Fatalf("accounts were merged or metadata regressed: %+v", entries)
	}
	entries[0].AccountLabel = "New A"
	entries[0].LastUsedAt = at.Add(time.Second).UnixMilli()
	if _, err := store.Import(entries); err != nil {
		t.Fatal(err)
	}
	store.Close()
	reloaded := New(path)
	defer reloaded.Close()
	got := reloaded.Summary()
	if len(got) != 2 || got[0].Calls != 4 || got[1].Calls != 2 || got[0].AccountLabel != "New A" || got[0].LastUsedAt != entries[0].LastUsedAt {
		t.Fatalf("account round trip lost values: %+v", got)
	}
}

func TestV2LedgerKeepsUnknownAccountWithoutBackfillDuplication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"entries":[{"date":"2026-11-01","model":"model","caller_id":"caller","prompt_tokens":10,"total_tokens":10,"calls":1,"cost":1.5}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(path)
	defer store.Close()
	if applied, err := store.Backfill([]BackfillItem{{Model: "model", Total: 10}}); err != nil || applied {
		t.Fatalf("existing ledger must not be backfilled: %v %v", applied, err)
	}
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, gmt8)
	store.RecordAccount("model", "caller", "account:a", "A", map[string]any{"prompt_tokens": 5}, at)
	entries := store.Summary()
	if len(entries) != 2 || entries[1].AccountID != "unknown" || entries[1].Calls != 1 || entries[1].Cost != 1.5 || entries[1].LastUsedAt != 0 {
		t.Fatalf("legacy totals or unknown timestamp changed: %+v", entries)
	}
}

func TestRequestRecorderUsesFinalAccountAndRecordsOnce(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	a := &auth.RequestAuth{UseConfigToken: true, AccountID: "first@example.com", CallerID: "caller:a"}
	at := time.Date(2026, 11, 1, 23, 59, 0, 0, gmt8)
	r := NewRequestRecorder(store, a, "model", at)
	r.Record(nil)
	a.AccountID = "final@example.com"
	a.Account = config.Account{Name: "Final", Token: "never-write-token", Password: "never-write-password"}
	usage := map[string]any{"input_tokens": 10, "output_tokens": 5}
	r.Record(usage)
	r.Record(usage)
	entries := store.Summary()
	if len(entries) != 1 || entries[0].Calls != 1 || entries[0].Date != "2026-11-01" || entries[0].LastUsedAt != at.UnixMilli() {
		t.Fatalf("wrong terminal accounting: %+v", entries)
	}
	id, label := usageAccount(a)
	if entries[0].AccountID != id || entries[0].AccountLabel != label || label != "Final · f***@example.com" {
		t.Fatalf("not attributed to the final account: %+v", entries)
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"final@example.com", "first@example.com", a.Account.Token, a.Account.Password} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("ledger contains credential/identifier %q", secret)
		}
	}
}

func TestDirectTokensDoNotMergeWithUnknownOrManagedAccounts(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	usage := map[string]any{"prompt_tokens": 10}
	for _, caller := range []string{"caller:a", "caller:b"} {
		a := &auth.RequestAuth{CallerID: caller, DeepSeekToken: "never-store-me"}
		NewRequestRecorder(store, a, "model", time.Now()).Record(usage)
	}
	entries := store.Summary()
	if len(entries) != 2 || entries[0].AccountID == entries[1].AccountID || !strings.HasPrefix(entries[0].AccountID, "direct:") {
		t.Fatalf("direct-token callers lost identity: %+v", entries)
	}
}

func TestAccountLabelRedactsLoginIdentifierUsedAsName(t *testing.T) {
	a := &auth.RequestAuth{UseConfigToken: true, AccountID: "login@example.com", Account: config.Account{Name: "Main login@example.com"}}
	_, label := usageAccount(a)
	if strings.Contains(label, a.AccountID) {
		t.Fatalf("login identifier escaped masking through name: %s", label)
	}
}

func TestPartialPromptOverrideKeepsReferenceFileContext(t *testing.T) {
	initial := promptcompat.StandardRequest{ResolvedModel: "model", FinalPrompt: "original", RefFileTokens: 37}
	withFiles := PartialUsage(initial, "", "answer", promptcompat.StandardRequest{PromptTokenText: "retry"})
	withoutFiles := PartialUsage(promptcompat.StandardRequest{ResolvedModel: "model"}, "", "answer", promptcompat.StandardRequest{PromptTokenText: "retry"})
	if withFiles["prompt_tokens"].(int)-withoutFiles["prompt_tokens"].(int) != 37 {
		t.Fatal("prompt-only override lost file token context")
	}
}
