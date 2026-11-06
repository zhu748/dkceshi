package promptcompat

import (
	"reflect"
	"strings"
	"testing"
)

func TestEmptyAssistantBoundarySurvivesPromptHistoryAndRebuild(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		messages := []any{map[string]any{"role": "user", "content": "TASK"}, map[string]any{"role": "assistant", "content": "", "id": "empty_assistant", "name": "agent", "prefix": prefix}}
		std, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, "")
		if err != nil {
			t.Fatal(err)
		}
		rebuilt, _ := std.BuildPromptFromMessages(std.Messages)
		for _, rendered := range []string{std.FinalPrompt, rebuilt, BuildOpenAIHistoryTranscript(messages)} {
			if !strings.Contains(rendered, `"id":"empty_assistant"`) || !strings.Contains(rendered, `"name":"agent"`) || !strings.Contains(rendered, `"prefix":`) {
				t.Fatalf("empty assistant boundary disappeared: %s", rendered)
			}
		}
		if len(NormalizeOpenAIMessagesForPrompt(messages, "")) != 2 || std.ContextManifest().LiveMessages[1].OmittedReason != "" {
			t.Fatal("empty assistant omitted from normalized context")
		}
		if prefix && !strings.HasSuffix(rebuilt, "[Assistant]:") {
			t.Fatal("empty prefill was closed")
		}
	}
}

func TestResponsesDoesNotMergeIndependentToolMessages(t *testing.T) {
	call := func(id string) map[string]any {
		return map[string]any{"role": "assistant", "tool_calls": []any{fidelityCall(id, `{}`)}}
	}
	for key, value := range map[string]any{"id": "message", "name": "agent", "prefix": false, "audio": map[string]any{"transcript": "spoken"}, "refusal": "refused"} {
		for _, position := range []int{0, 1} {
			first, second := call("c1"), call("c2")
			input := []any{first, second}
			input[position].(map[string]any)[key] = value
			got := NormalizeResponsesInputAsMessages(input)
			if len(got) != 2 || !reflect.DeepEqual(got[position].(map[string]any)[key], value) {
				t.Fatalf("metadata %s at position %d lost: %#v", key, position, got)
			}
		}
	}
	if got := NormalizeResponsesInputAsMessages([]any{call("c1"), call("c2")}); len(got) != 1 {
		t.Fatal("anonymous tool items no longer form a batch")
	}
}

func TestResponsesPreservesNamedUserAndEmptyPrefill(t *testing.T) {
	input := []any{map[string]any{"role": "user", "name": "alice", "id": "u1", "content": "TASK"}, map[string]any{"role": "assistant", "prefix": true, "id": "a1"}}
	got := NormalizeResponsesInputAsMessages(input)
	if len(got) != 2 || got[0].(map[string]any)["name"] != "alice" || got[0].(map[string]any)["id"] != "u1" {
		t.Fatalf("message identity lost: %#v", got)
	}
	rendered := RenderPromptText(got, false)
	if !strings.Contains(rendered, `"name":"alice"`) || !strings.Contains(rendered, `"id":"a1"`) {
		t.Fatalf("identity absent from final prompt: %s", rendered)
	}
}

func TestRecognizedMalformedTextRejectedAcrossOpenAIInputs(t *testing.T) {
	for _, block := range []any{
		map[string]any{"type": "text", "text": map[string]any{"instruction": "LOST"}},
		map[string]any{"type": "input_text", "text": 42},
		map[string]any{"type": "output_text"},
		map[string]any{"type": "text", "text": "KEPT", "content": []any{"LOST"}},
		map[string]any{"type": "thinking", "thinking": 42},
	} {
		message := map[string]any{"role": "user", "content": []any{block, map[string]any{"type": "text", "text": "VALID"}}}
		for _, field := range []string{"messages", "input"} {
			request := map[string]any{"model": "deepseek-v4.1-flash", field: []any{message}}
			var err error
			if field == "messages" {
				_, err = NormalizeOpenAIChatRequest(nil, request, "")
			} else {
				_, err = NormalizeOpenAIResponsesRequest(nil, request, "")
			}
			if err == nil {
				t.Fatalf("malformed known block accepted through %s: %#v", field, block)
			}
		}
	}
	message := map[string]any{"role": "tool", "content": map[string]any{"type": "text", "text": 42}}
	if _, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}}, ""); err != nil {
		t.Fatalf("opaque tool business JSON was validated as a content block: %v", err)
	}
}

func TestInputContextRetainsPreAdapterEvidence(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "instructions": "PRIVATE_INSTRUCTION", "input": []any{map[string]any{"role": "user", "content": "PRIVATE_BODY", "name": "alice"}}}
	want := InputFieldDigests(request)
	std, err := NormalizeOpenAIResponsesRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	request["input"] = "mutated"
	std.SourceContext = std.ContextMessageDigests()
	std.Messages = []any{map[string]any{"role": "user", "content": "continue"}}
	std.HistoryText, std.CurrentInputFileApplied = "file context", true
	manifest := std.ContextManifest()
	if !reflect.DeepEqual(manifest.InputFields, want) || len(manifest.SourceMessages) != 2 {
		t.Fatalf("pre-adapter evidence changed across externalization: %+v", manifest)
	}
	manifest.InputFields[0].SHA256 = "mutated"
	if reflect.DeepEqual(manifest.InputFields, std.InputContext) {
		t.Fatal("manifest aliases request input evidence")
	}
}

func TestOpenAIInterleavedReasoningRetainsSourceOrder(t *testing.T) {
	message := map[string]any{"role": "assistant", "content": []any{
		map[string]any{"type": "output_text", "text": "ORDER_1"},
		map[string]any{"type": "reasoning", "text": "ORDER_2"},
		map[string]any{"type": "output_text", "text": "ORDER_3"},
		map[string]any{"type": "thinking", "thinking": "ORDER_4"},
	}}
	for _, messages := range [][]any{{message}, NormalizeResponsesInputAsMessages([]any{message})} {
		for _, rendered := range []string{RenderPromptText(messages, true), BuildOpenAIHistoryTranscript(messages)} {
			previous := -1
			for _, token := range []string{"ORDER_1", "ORDER_2", "ORDER_3", "ORDER_4"} {
				index := strings.Index(rendered, token)
				if index <= previous || strings.Count(rendered, token) != 1 {
					t.Fatalf("reasoning reordered or duplicated: %s", rendered)
				}
				previous = index
			}
		}
	}
}
