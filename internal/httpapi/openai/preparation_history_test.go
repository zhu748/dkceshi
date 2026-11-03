package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/chathistory"
	dsclient "ds2api/internal/deepseek/client"
)

func TestUploadFailuresPersistHistoryBeforeCompletion(t *testing.T) {
	for _, surface := range []string{"chat", "responses"} {
		for _, stage := range []string{"inline", "history"} {
			for _, stream := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/stream=%v", surface, stage, stream), func(t *testing.T) {
					ds := &inlineUploadDSStub{uploadErr: &dsclient.FileRateLimitError{Op: "upload file", Code: 40029}}
					history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
					cfg := mockOpenAIConfig{}
					if stage == "history" {
						cfg.currentInputEnabled = true
						cfg.currentInputMin = 1
					}
					h := &openAITestSurface{Store: cfg, Auth: streamStatusAuthStub{}, DS: ds, ChatHistory: history}
					content := `"please continue"`
					if stage == "inline" {
						content = `[{"type":"text","text":"please continue"},{"type":"input_file","filename":"test.txt","file_data":"QUJDRA=="}]`
					}
					body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":%s}],"stream":%v}`, content, stream)
					req := httptest.NewRequest(http.MethodPost, "/v1/"+surface, strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer direct-token")
					rec := httptest.NewRecorder()
					if surface == "chat" {
						h.ChatCompletions(rec, req)
					} else {
						h.responsesHandler().Responses(rec, req)
					}
					if rec.Code != 429 || !strings.Contains(rec.Body.String(), "40029") {
						t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
					}
					if ds.completionReq != nil {
						t.Fatal("started completion despite upload failure")
					}
					snapshot, err := history.Snapshot()
					if err != nil || len(snapshot.Items) != 1 {
						t.Fatalf("missing failure history: %+v %v", snapshot, err)
					}
					entry, err := history.Get(snapshot.Items[0].ID)
					if err != nil || entry.Status != "error" || entry.StatusCode != 429 || !strings.Contains(entry.Error, "40029") || !strings.Contains(entry.UserInput, "please continue") {
						t.Fatalf("failure not archived: %+v %v", entry, err)
					}
					if strings.Contains(entry.FinalPrompt, "QUJDRA==") {
						t.Fatal("base64 leaked into diagnostic prompt")
					}
				})
			}
		}
	}
}
