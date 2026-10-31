package completionruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestNonStreamPromptEchoRegeneratesWithFeedback(t *testing.T) {
	const prefix = "<｜System｜>custom thinking instruction"
	response := func(text string) *http.Response {
		data, err := json.Marshal(map[string]any{"p": "response/content", "v": text, "response_message_id": 17})
		if err != nil {
			t.Fatal(err)
		}
		return sseHTTPResponse(http.StatusOK, "data: "+string(data), "data: [DONE]")
	}
	for _, exhaust := range []bool{false, true} {
		next := "正常回答"
		if exhaust {
			next = prefix
		}
		ds := &fakeDeepSeekCaller{responses: []*http.Response{response(prefix), response(next)}}
		req := promptcompat.StandardRequest{FinalPrompt: prefix + "\n\n[User]:continue", PromptTokenText: prefix, ResolvedModel: "deepseek-v4.1-flash", Thinking: true}
		result, err := ExecuteNonStreamWithRetry(context.Background(), ds, &auth.RequestAuth{}, req, Options{RetryEnabled: true})
		if exhaust {
			if err == nil || err.Code != "upstream_prompt_echo" || err.Status != 502 || result.Turn.Text != "" {
				t.Fatalf("exhausted echo reported success: turn=%+v err=%+v", result.Turn, err)
			}
		} else if err != nil || result.Turn.Text != next {
			t.Fatalf("healthy retry lost: turn=%+v err=%+v", result.Turn, err)
		}
		if len(ds.payloads) != 2 || ds.payloads[1]["parent_message_id"] != 17 || !strings.HasSuffix(ds.payloads[1]["prompt"].(string), PromptEchoRetrySuffix) {
			t.Fatalf("retry did not retain parent or feedback: %v", ds.payloads)
		}
	}
}
