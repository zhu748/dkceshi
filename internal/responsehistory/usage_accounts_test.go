package responsehistory

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/assistantturn"
	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/usagestats"
	"ds2api/internal/util"
)

func TestSharedHistoryAttributesEffectiveCallsWithoutHistory(t *testing.T) {
	for _, surface := range []string{"openai.responses", "claude.messages", "gemini.generate_content"} {
		for _, terminal := range []string{"success", "stopped", "error"} {
			t.Run(surface+"/"+terminal, func(t *testing.T) {
				store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
				defer store.Close()
				a := &auth.RequestAuth{UseConfigToken: true, AccountID: "first@example.com", CallerID: "caller:a"}
				s := Start(StartParams{Usage: store, Auth: a, Surface: surface,
					Request:  httptest.NewRequest(http.MethodPost, "/", nil),
					Standard: promptcompat.StandardRequest{ResolvedModel: "upstream", ResponseModel: "alias", FinalPrompt: "hello"},
				})
				a.AccountID = "final@example.com"
				turn := assistantturn.Turn{Text: "answer", Usage: assistantturn.Usage{InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2, TotalTokens: 15}}
				switch terminal {
				case "success":
					s.SuccessTurn(http.StatusOK, turn, GenericUsage(turn))
					s.SuccessTurn(http.StatusOK, turn, GenericUsage(turn))
				case "stopped":
					s.Stopped("thinking", "answer", "stopped")
					s.Stopped("thinking", "answer", "stopped")
				case "error":
					s.Error(502, "failed", "error", "", "")
				}
				entries := store.Summary()
				if terminal == "error" {
					if len(entries) != 0 {
						t.Fatal("failure must not increment effective calls")
					}
					return
				}
				if len(entries) != 1 || entries[0].Calls != 1 || entries[0].Model != "upstream" || entries[0].AccountLabel != "f***@example.com" {
					t.Fatalf("bad shared terminal accounting: %+v", entries)
				}
				if terminal == "success" && entries[0].ReasoningTokens != 2 {
					t.Fatal("reasoning must remain visible independently of protocol usage formatting")
				}
			})
		}
	}
}

func TestStoppedUsageCountsCurrentPromptAndReferenceFiles(t *testing.T) {
	store := usagestats.New(filepath.Join(t.TempDir(), "usage.json"))
	defer store.Close()
	initial := promptcompat.StandardRequest{ResolvedModel: "deepseek-v4.1-flash", FinalPrompt: "placeholder", PromptTokenText: "original full context", RefFileTokens: 17}
	s := Start(StartParams{Usage: store, Auth: &auth.RequestAuth{CallerID: "caller"}, Request: httptest.NewRequest("POST", "/", nil), Standard: initial})
	current := promptcompat.StandardRequest{PromptTokenText: strings.Repeat("retry context ", 100), RefFileTokens: 37}
	s.Stopped("thinking", "answer", "stopped", current)
	expected := util.CountPromptTokens(current.PromptTokenText, initial.ResolvedModel) + 37
	entries := store.Summary()
	if len(entries) != 1 || entries[0].PromptTokens != int64(expected) || entries[0].PromptTokens <= 37 {
		t.Fatalf("partial usage lost current context/files: %+v", entries)
	}
}
