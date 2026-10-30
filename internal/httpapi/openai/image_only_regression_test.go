package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageOnlyRequestsKeepUserTurnAndFileReference(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct {
			name, input string
			responses   bool
			uploads     int
		}{
			{"chat-inline", `"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}]`, false, 1},
			{"chat-reference", `"messages":[{"role":"user","content":[{"type":"input_image","file_id":"file-existing"}]}]`, false, 0},
			{"chat-follow-up", `"messages":[{"role":"user","content":"earlier"},{"role":"assistant","content":"previous reply"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}]`, false, 1},
			{"responses-part", `"input":[{"type":"input_image","image_url":"data:image/png;base64,QUJDRA=="}]`, true, 1},
			{"responses-message", `"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,QUJDRA=="}]}]`, true, 1},
			{"responses-object", `"input":{"type":"input_image","image_url":"data:image/png;base64,QUJDRA=="}`, true, 1},
			{"responses-reference", `"input":[{"type":"input_image","file_id":"file-existing"}]`, true, 0},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				ds := &inlineUploadDSStub{}
				h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, DS: ds}
				body := fmt.Sprintf(`{"model":"deepseek-v4.1-flash",%s,"stream":%t}`, tc.input, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer test-token")
				rec := httptest.NewRecorder()
				if tc.responses {
					h.responsesHandler().Responses(rec, req)
				} else {
					h.ChatCompletions(rec, req)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("%d %s", rec.Code, rec.Body.String())
				}
				refs, _ := ds.completionReq["ref_file_ids"].([]any)
				if len(refs) != 1 || len(ds.uploadCalls) != tc.uploads {
					t.Fatalf("refs=%v uploads=%d", refs, len(ds.uploadCalls))
				}
				prompt, _ := ds.completionReq["prompt"].(string)
				if !strings.Contains(prompt, "[User]:[Attachment reference]") || !strings.HasSuffix(prompt, "[Assistant]:") || strings.Contains(prompt, "QUJDRA==") {
					t.Fatalf("image turn lost or file data leaked: %q", prompt)
				}
			})
		}
	}
}
