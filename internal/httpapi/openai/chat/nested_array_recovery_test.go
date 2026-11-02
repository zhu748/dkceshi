package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/jsonvalue"
	"ds2api/internal/sse"
)

func nestedArrayFixture(t *testing.T) (prefix, raw string, expected any) {
	t.Helper()
	data, err := os.ReadFile("../../../../tests/compat/fixtures/schema_recovery/array_nested_eof.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Prefix, Raw string
		Expected    json.RawMessage
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := jsonvalue.Unmarshal(fixture.Expected, &expected); err != nil {
		t.Fatal(err)
	}
	return fixture.Prefix, fixture.Raw, expected
}

func assertNestedQuestionArguments(t *testing.T, call map[string]any, expected any) {
	t.Helper()
	fn, _ := call["function"].(map[string]any)
	if fn["name"] != "ask_user_question" || asString(call["id"]) == "" {
		t.Fatal("tool identity lost")
	}
	var input map[string]any
	if err := jsonvalue.Unmarshal([]byte(asString(fn["arguments"])), &input); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input["questions"], expected) {
		t.Fatalf("question arguments changed: %#v", input)
	}
}

func TestNestedQuestionArrayRecoveryPreservesProseAndStreamsOneCall(t *testing.T) {
	prefix, value, expected := nestedArrayFixture(t)
	std := questionArrayRequest(t)
	raw := prefix + questionArrayMarkup(value)
	for _, size := range []int{1, 17, len(raw)} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt := newChatStreamRuntime(rec, http.NewResponseController(rec), true, "id", 1,
				std.ResponseModel, std.FinalPrompt, false, false, true, std.ToolNames, std.ToolsRaw, std.ToolChoice, true, true)
			rt.toolSieve.Marker = questionArrayMarker
			rt.toolCallRepair = func(context.Context, string) (string, error) {
				t.Fatal("closing containers must not invoke LLM repair")
				return "", nil
			}
			for i := 0; i < len(raw); i += size {
				rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
			}
			if !rt.finalize("stop", true) || rt.finalErrorCode != "" {
				t.Fatalf("local recovery failed or requested regeneration: %s", rt.finalErrorCode)
			}
			frames, done := parseSSEDataFrames(t, rec.Body.String())
			if !done || streamFinishReason(frames) != "tool_calls" || collectStreamContent(frames) != prefix {
				t.Fatalf("prose lost, duplicated or contaminated: %s", rec.Body.String())
			}
			var calls []map[string]any
			for _, frame := range frames {
				choices, _ := frame["choices"].([]any)
				for _, item := range choices {
					choice := item.(map[string]any)
					delta, _ := choice["delta"].(map[string]any)
					batch, _ := delta["tool_calls"].([]any)
					for _, call := range batch {
						calls = append(calls, call.(map[string]any))
					}
				}
			}
			if len(calls) != 1 || strings.Contains(rec.Body.String(), `"error"`) {
				t.Fatalf("unexpected call delivery: %s", rec.Body.String())
			}
			assertNestedQuestionArguments(t, calls[0], expected)
		})
	}
}

func TestVercelNestedQuestionArrayUsesLocalRecovery(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "nested-array-test-secret")
	prefix, value, expected := nestedArrayFixture(t)
	std := questionArrayRequest(t)
	h := &Handler{}
	lease := h.holdStreamLease(&auth.RequestAuth{CallerID: "owner"}, std, "session")
	data, err := json.Marshal(map[string]any{"lease_id": lease, "content": prefix + questionArrayMarkup(value)})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
	req.Header.Set("X-Ds2-Internal-Token", "nested-array-test-secret")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Node final validation failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	out := decodeJSONBody(t, rec.Body.String())
	calls, _ := out["tool_calls"].([]any)
	if out["content"] != prefix || len(calls) != 1 {
		t.Fatalf("Node final validation lost prose or tool call: %#v", out)
	}
	assertNestedQuestionArguments(t, calls[0].(map[string]any), expected)
}
