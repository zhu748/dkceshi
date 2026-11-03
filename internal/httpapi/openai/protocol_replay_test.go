package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/httpapi/claude"
	"ds2api/internal/httpapi/gemini"
	"github.com/go-chi/chi/v5"
)

func TestNativeProtocolAttachmentReplay(t *testing.T) {
	for _, protocol := range []string{"responses", "claude", "gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", protocol, stream), func(t *testing.T) {
				t.Setenv("DS2API_CONFIG_JSON", `{"keys":["key"],"input_file":{"enabled":true,"min_chars":1},"accounts":[{"email":"a@example.com","token":"ta"},{"email":"b@example.com","token":"tb"}]}`)
				s := config.LoadStore()
				p := account.NewPool(s)
				defer p.Close()
				r := auth.NewResolver(s, p, func(context.Context, config.Account) (string, error) { return "token", nil })
				ds := &poolFailoverDS{resolver: r, phase: "completion"}
				var body string
				var handle http.HandlerFunc
				switch protocol {
				case "responses":
					body = fmt.Sprintf(`{"model":"deepseek-v4.1-flash","stream":%v,"input":[{"role":"user","content":[{"type":"input_text","text":"continue"},{"type":"input_file","filename":"one.txt","file_data":"QUJDRA=="}]}]}`, stream)
					h := &openAITestSurface{Store: s, Auth: r, DS: ds}
					handle = h.responsesHandler().Responses
				case "claude":
					body = fmt.Sprintf(`{"model":"deepseek-v4.1-flash","max_tokens":100,"stream":%v,"system":"RULES","messages":[{"role":"user","content":[{"type":"text","text":"continue"},{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"QUJDRA=="}}]}]}`, stream)
					h := &claude.Handler{Store: s, Auth: r, DS: ds}
					handle = h.Messages
				case "gemini":
					body = `{"contents":[{"role":"user","parts":[{"text":"continue"},{"inlineData":{"mimeType":"text/plain","data":"QUJDRA=="}}]}]}`
					h := &gemini.Handler{Store: s, Auth: r, DS: ds}
					handle = h.GenerateContent
					if stream {
						handle = h.StreamGenerateContent
					}
				}
				req := httptest.NewRequest("POST", "/v1/test", strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer key")
				route := chi.NewRouteContext()
				route.URLParams.Add("model", "deepseek-v4.1-flash")
				req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
				rec := httptest.NewRecorder()
				handle(rec, req)
				if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ok") {
					t.Fatalf("replay failed: %d %s", rec.Code, rec.Body.String())
				}
				if len(ds.replyOwners) != 2 || ds.replyOwners[0] == ds.replyOwners[1] {
					t.Fatalf("wrong owners: %v", ds.replyOwners)
				}
				for _, id := range ds.completionReq["ref_file_ids"].([]any) {
					if !strings.HasPrefix(fmt.Sprint(id), ds.replyOwners[1]+"-") {
						t.Fatalf("mixed references: %v", id)
					}
				}
				if p.Status()["in_use"] != 0 {
					t.Fatal("leaked slot")
				}
			})
		}
	}
}
