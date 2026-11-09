package promptcompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTextDocumentWithReferenceSurvivesRebuilds(t *testing.T) {
	document := map[string]any{"type": "document", "file_id": "file-existing", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": "  DOCUMENT_BODY\r\n"}, "description": "DOCUMENT_DESCRIPTION"}
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{document}}}}
	before, _ := json.Marshal(request)
	standard, err := NormalizeOpenAIChatRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		rebuilt, _ := standard.BuildPromptFromMessages(standard.Messages)
		for _, text := range []string{standard.FinalPrompt, rebuilt, BuildOpenAIHistoryTranscript(standard.Messages)} {
			for _, want := range []string{"DOCUMENT_BODY", "DOCUMENT_DESCRIPTION", "file-existing"} {
				if strings.Count(text, want) != 1 {
					t.Fatalf("iteration %d: missing/duplicated %s: %s", i, want, text)
				}
			}
		}
		if !reflect.DeepEqual(standard.RefFileIDs, []string{"file-existing"}) {
			t.Fatalf("reference lost: %v", standard.RefFileIDs)
		}
		standard, err = NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
		if err != nil {
			t.Fatal(err)
		}
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("caller document changed")
	}
}

func TestResponsesNativeToolIdentitySurvivesChatBridge(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{
		map[string]any{"type": "function_call", "id": "fc-source", "call_id": "call-link", "name": "lookup", "arguments": `{"n":9007199254740993}`},
		map[string]any{"type": "function_call_output", "id": "out-source", "call_id": "call-link", "output": "RESULT"},
	}}
	standard, err := NormalizeOpenAIResponsesRequest(nil, request, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
			for _, want := range []string{"fc-source", "call-link", "out-source", "9007199254740993", "RESULT"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s after iteration %d: %s", want, i, text)
				}
			}
		}
		standard, err = NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWhitespaceReasoningSurvivesResponsesAndChat(t *testing.T) {
	for _, reasoning := range []any{" \r\n\t", []any{map[string]any{"type": "thinking", "thinking": " \r\n\t"}}} {
		request := map[string]any{"model": "deepseek-v4.1-flash", "input": []any{map[string]any{"role": "assistant", "reasoning_content": reasoning}}}
		standard, err := NormalizeOpenAIResponsesRequest(nil, request, "")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
				if !strings.Contains(text, "[reasoning_content]\n \r\n\t\n[/reasoning_content]") {
					t.Fatalf("whitespace reasoning lost: %q", text)
				}
			}
			standard, err = NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestFullMessageDigestIncludesIdentityAndExtensionContext(t *testing.T) {
	digest := func(id, note string) ContextMessageDigest {
		request := StandardRequest{Messages: []any{map[string]any{"role": "user", "content": "UNCHANGED_BODY", "id": id, "metadata": map[string]any{"note": note}}}}
		return request.ContextMessageDigests()[0]
	}
	before := digest("id-a", "note-a")
	for _, after := range []ContextMessageDigest{digest("id-b", "note-a"), digest("id-a", "note-b")} {
		if before.PromptContent != after.PromptContent || before.NormalizedMessage == after.NormalizedMessage {
			t.Fatal("complete representation fingerprint did not distinguish metadata from the body")
		}
	}
}

func TestAdjacentResponsesCallsRetainPerOccurrenceItemIDs(t *testing.T) {
	items := []any{
		map[string]any{"type": "function_call", "id": "fc-first", "call_id": "call-first", "name": "lookup", "arguments": "{}"},
		map[string]any{"type": "function_call", "id": "fc-second", "call_id": "call-second", "name": "lookup", "arguments": "{}"},
	}
	messages := NormalizeResponsesInputAsMessages(items)
	if len(messages) != 1 || len(messages[0].(map[string]any)["tool_calls"].([]any)) != 2 {
		t.Fatal("contextual identity changed adjacent call grouping")
	}
	transcript := BuildOpenAIHistoryTranscript(messages)
	if strings.Count(transcript, "fc-first") != 1 || strings.Count(transcript, "fc-second") != 1 || strings.Index(transcript, "fc-first") > strings.Index(transcript, "fc-second") {
		t.Fatal("native item identity lost its occurrence or source order")
	}
}
