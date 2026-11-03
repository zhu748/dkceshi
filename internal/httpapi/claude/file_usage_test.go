package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	dsclient "ds2api/internal/deepseek/client"
)

type claudeUsageDS struct {
	claudeCurrentInputDS
	tokens int
}

func (d *claudeUsageDS) FetchUploadedFiles(_ context.Context, _ *auth.RequestAuth, ids []string) ([]*dsclient.UploadFileResult, error) {
	files := make([]*dsclient.UploadFileResult, 0, len(ids))
	for _, id := range ids {
		tokens := d.tokens
		files = append(files, &dsclient.UploadFileResult{ID: id, Status: "SUCCESS", TokenUsage: &tokens})
	}
	return files, nil
}

func TestClaudeMessagesAndCountTokensIncludeReferencedFileUsage(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		count := func(tokens int) int {
			ds := &claudeUsageDS{tokens: tokens}
			h := &Handler{Store: claudeHistoryConfig{}, Auth: claudeCurrentInputAuth{}, DS: ds}
			body := `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"describe"}],"file_ids":["file-existing","file-existing"]}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			if strings.HasSuffix(path, "count_tokens") {
				h.CountTokens(rec, req)
			} else {
				h.Messages(rec, req)
			}
			if rec.Code != 200 {
				t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
			}
			if len(ds.uploads) != 0 {
				t.Fatal("existing file was uploaded again")
			}
			var obj map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &obj); err != nil {
				t.Fatal(err)
			}
			if usage, ok := obj["usage"].(map[string]any); ok {
				obj = usage
			}
			v, ok := obj["input_tokens"].(float64)
			if !ok {
				t.Fatal(fmt.Sprint(obj))
			}
			return int(v)
		}
		if delta := count(954) - count(0); delta != 954 {
			t.Fatalf("%s file token delta=%d", path, delta)
		}
	}
}
