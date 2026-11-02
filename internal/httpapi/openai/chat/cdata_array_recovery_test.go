package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

const questionArrayMarker = "ZBBZUX"
const questionArrayMissingClose = `[{"id":"target","question":"安装到哪里？","options":[{"label":"Codex"}]},{"id":"enable","question":"是否启用？"}`

func questionArrayRequest(t *testing.T) promptcompat.StandardRequest {
	t.Helper()
	var tools []any
	if err := json.Unmarshal([]byte(`[{
  "type": "function",
  "function": {
    "name": "ask_user_question",
    "parameters": {
      "type": "object", "required": ["questions"],
      "properties": {
        "questions": {
          "type": "array",
          "items": {
            "type": "object", "required": ["id", "question"],
            "properties": {
              "id": {"type": "string"}, "question": {"type": "string"},
              "options": {"type": "array", "items": {
                "type": "object", "required": ["label"], "properties": {"label": {"type": "string"}}
              }}
            }
          }
        }
      }
    }
  }
}]`), &tools); err != nil {
		t.Fatal(err)
	}
	std, err := promptcompat.NormalizeOpenAIChatRequest(nil, map[string]any{
		"model": "deepseek-v4.1-flash", "tools": tools,
		"messages": []any{map[string]any{"role": "user", "content": "继续安装"}},
	}, "", questionArrayMarker)
	if err != nil {
		t.Fatal(err)
	}
	return std
}

func questionArrayMarkup(value string) string {
	return `<|ZBBZUX|tool_calls><|ZBBZUX|invoke name="ask_user_question"><|ZBBZUX|parameter name="questions"><![CDATA[` + value + `]]></|ZBBZUX|parameter></|ZBBZUX|invoke></|ZBBZUX|tool_calls>`
}

func checkRecoveredQuestionCall(t *testing.T, call map[string]any, std promptcompat.StandardRequest) {
	t.Helper()
	fn, _ := call["function"].(map[string]any)
	if fn["name"] != "ask_user_question" || asString(call["id"]) == "" {
		t.Fatalf("lost tool identity: %#v", call)
	}
	var got, want any
	if err := json.Unmarshal([]byte(asString(fn["arguments"])), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"questions":`+questionArrayMissingClose+`]}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("question arguments changed: got=%#v want=%#v", got, want)
	}
	// The client returns the result using the delivered call ID. The next
	// agent request must preserve both the recovered array and this pairing.
	messages := append([]any{}, std.Messages...)
	messages = append(messages,
		map[string]any{"role": "assistant", "tool_calls": []any{call}},
		map[string]any{"role": "tool", "tool_call_id": call["id"], "name": fn["name"], "content": `{"target":"Codex","enable":true}`},
	)
	next, err := promptcompat.NormalizeOpenAIChatRequest(nil, map[string]any{
		"model": "deepseek-v4.1-flash", "tools": std.ToolsRaw, "messages": messages,
	}, "", questionArrayMarker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next.FinalPrompt, `call_id="`+asString(call["id"])+`"`) || !strings.Contains(next.FinalPrompt, `"id":"target"`) {
		t.Fatal("agent continuation lost the call ID or recovered questions")
	}
}

func TestQuestionArrayRecoveryStreamsValidatedCallAndContinuesAgent(t *testing.T) {
	std := questionArrayRequest(t)
	raw := questionArrayMarkup(questionArrayMissingClose)
	for _, size := range []int{1, 7, len(raw)} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1,
				std.ResponseModel, std.FinalPrompt, false, false, true, std.ToolNames, std.ToolsRaw, std.ToolChoice, true, true)
			rt.toolSieve.Marker = questionArrayMarker
			for i := 0; i < len(raw); i += size {
				rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
				frames, _ := parseSSEDataFrames(t, rec.Body.String())
				if streamHasToolCallsDelta(frames) {
					t.Fatal("tool call escaped before full schema validation")
				}
			}
			if !rt.finalize("stop", false) || rt.finalErrorCode != "" {
				t.Fatalf("recovered array failed: code=%s body=%s", rt.finalErrorCode, rec.Body.String())
			}
			frames, _ := parseSSEDataFrames(t, rec.Body.String())
			if streamFinishReason(frames) != "tool_calls" || collectStreamContent(frames) != "" {
				t.Fatalf("wrong delivery: %s", rec.Body.String())
			}
			var delivered []map[string]any
			for _, frame := range frames {
				for _, item := range frame["choices"].([]any) {
					choice := item.(map[string]any)
					delta, _ := choice["delta"].(map[string]any)
					calls, _ := delta["tool_calls"].([]any)
					for _, call := range calls {
						delivered = append(delivered, call.(map[string]any))
					}
				}
			}
			if len(delivered) != 1 {
				t.Fatalf("expected one call, got %d", len(delivered))
			}
			checkRecoveredQuestionCall(t, delivered[0], std)
		})
	}
}

func TestVercelQuestionArrayRecoveryUsesSharedSchemaValidation(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "array-validation-secret")
	std := questionArrayRequest(t)
	h := &Handler{}
	lease := h.holdStreamLease(&auth.RequestAuth{CallerID: "owner"}, std, "session")
	for _, tc := range []struct {
		value  string
		status int
	}{
		{questionArrayMissingClose, http.StatusOK},
		{`[{"id":"q"}`, http.StatusUnprocessableEntity},
	} {
		data, err := json.Marshal(map[string]any{"lease_id": lease, "content": questionArrayMarkup(tc.value)})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
		req.Header.Set("X-Ds2-Internal-Token", "array-validation-secret")
		rec := httptest.NewRecorder()
		h.ChatCompletions(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		out := decodeJSONBody(t, rec.Body.String())
		if tc.status == http.StatusOK {
			calls, _ := out["tool_calls"].([]any)
			if len(calls) != 1 {
				t.Fatalf("wrong calls: %#v", out)
			}
			checkRecoveredQuestionCall(t, calls[0].(map[string]any), std)
		} else if _, ok := out["tool_calls"]; ok {
			t.Fatal("invalid call escaped schema validation")
		}
	}
}
