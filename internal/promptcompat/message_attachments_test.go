package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageAttachmentsSurviveRenderingHistoryAndChatBridge(t *testing.T) {
	for _, key := range AttachmentInputKeys() {
		for _, role := range []string{"user", "assistant", "tool", "system", "developer"} {
			source := map[string]any{"role": role, "id": "source-id", "content": "  SOURCE\r\n", key: []any{
				map[string]any{"type": "input_file", "file_id": "file-source", "filename": "source.txt", "description": "ATTACHMENT_CONTEXT"},
			}}
			request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{source}}
			before, _ := json.Marshal(request)
			standard, err := NormalizeOpenAIChatRequest(nil, request, "")
			if err != nil {
				t.Fatal(err)
			}
			bridged, err := NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages), bridged.FinalPrompt, RenderPromptText([]any{source}, false)} {
				for _, want := range []string{"  SOURCE\r\n", "ATTACHMENT_CONTEXT", "source-id", "file-source", "source.txt"} {
					if strings.Count(text, want) != 1 {
						t.Fatalf("%s/%s missing or duplicated %q: %s", role, key, want, text)
					}
				}
			}
			if len(standard.RefFileIDs) != 1 || len(standard.ContextManifest().Omissions) != 0 {
				t.Fatal("message attachments lost their references or serializer")
			}
			after, _ := json.Marshal(request)
			if string(before) != string(after) {
				t.Fatal("attachment normalization mutated the caller")
			}
		}
	}
}

func TestResponsesMessageAttachmentsKeepEmptyBodiesAndItemBoundaries(t *testing.T) {
	for _, kind := range []string{"message", "input_message", "input_text", "output_text"} {
		item := map[string]any{"type": kind, "role": "assistant", "id": "prefill-id", "prefix": true, "attachments": []any{"file-a"}}
		if strings.HasSuffix(kind, "text") {
			item["text"] = "  PREFILL\r\n"
		} else {
			item["content"] = "  PREFILL\r\n"
		}
		standard, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "input": []any{item}}, "")
		if err != nil || strings.Count(standard.FinalPrompt, "file-a") != 1 || strings.Count(standard.FinalPrompt, "  PREFILL\r\n") != 1 {
			t.Fatalf("%s lost/duplicated attachment context: %v %s", kind, err, standard.FinalPrompt)
		}
		if !strings.HasSuffix(standard.FinalPrompt, "  PREFILL\r\n") || strings.Count(standard.FinalPrompt, "prefill-id") != 1 {
			t.Fatal("attachment metadata displaced prefill or duplicated identity")
		}
	}
	message := map[string]any{"role": "tool", "content": nil, "files": []any{"file-a"}}
	text := RenderPromptText([]any{message}, false)
	if !strings.Contains(text, "[Tool]:null") || !strings.Contains(text, "file-a") {
		t.Fatal("null tool result was replaced by its attachment")
	}
}

func TestRolelessResponsesCannotDiscardToolHistory(t *testing.T) {
	for _, role := range []any{nil, "", "  "} {
		for _, key := range []string{"tool_calls", "function_call"} {
			call := map[string]any{"id": "call-a", "name": "read", "arguments": "{}"}
			item := map[string]any{"type": "message", "role": role, "content": "context", key: call}
			if key == "tool_calls" {
				item[key] = []any{call}
			}
			if err := ValidateResponsesRequestContext(map[string]any{"input": []any{item}}); err == nil {
				t.Fatalf("roleless %s was silently discarded", key)
			}
		}
	}
	if err := ValidateResponsesRequestContext(map[string]any{"input": []any{map[string]any{"content": "normal roleless text"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestMessageAttachmentPreviewOmitsBinaryAndRejectsUnsupportedMedia(t *testing.T) {
	for _, key := range AttachmentInputKeys() {
		message := map[string]any{"role": "user", "content": "TASK", key: []any{map[string]any{"file_data": "data:text/plain;base64,UFJJVkFURQ==", "description": "DESCRIPTION"}}}
		request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}}
		standard, err := NormalizeOpenAIChatRequest(nil, request, "")
		if err != nil || strings.Contains(standard.FinalPrompt, "UFJJVkFURQ==") || !strings.Contains(standard.FinalPrompt, "DESCRIPTION") {
			t.Fatalf("attachment preview exposed bytes or lost text: %v", err)
		}
		manifest := (StandardRequest{Messages: []any{message}}).ContextManifest()
		if len(manifest.Omissions) != 1 || manifest.Omissions[0].Field != "/"+key+"/0/file_data" {
			t.Fatalf("attachment binary omission not recorded: %+v", manifest.Omissions)
		}
		message[key] = []any{map[string]any{"type": "input_audio", "data": "bytes"}}
		if _, err := NormalizeOpenAIChatRequest(nil, request, ""); err == nil {
			t.Fatal("message attachment bypassed media validation")
		}
	}
}

func TestNativeAttachmentContainersKeepSourceOrderAcrossFieldSpellings(t *testing.T) {
	message := map[string]any{"role": "tool", "content": "RESULT", "files": []any{map[string]any{"file_id": "initial", "description": "INITIAL_ATTACHMENT"}}}
	AppendMessageAttachments(message,
		map[string]any{"files": []any{map[string]any{"file_id": "inner", "description": "INNER_ATTACHMENT"}}},
		map[string]any{"attachments": []any{map[string]any{"file_id": "outer", "description": "OUTER_ATTACHMENT"}}})
	text := RenderPromptText([]any{message}, false)
	if strings.Index(text, "INITIAL_ATTACHMENT") > strings.Index(text, "INNER_ATTACHMENT") || strings.Index(text, "INNER_ATTACHMENT") > strings.Index(text, "OUTER_ATTACHMENT") {
		t.Fatal("attachment field spellings reordered native source containers")
	}
}
