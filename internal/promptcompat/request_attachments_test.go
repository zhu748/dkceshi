package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestAttachmentContextSurvivesPromptHistoryAndBridge(t *testing.T) {
	for _, key := range AttachmentInputKeys() {
		raw := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
			map[string]any{"role": "system", "content": "SYSTEM"},
			map[string]any{"role": "user", "id": "source-user", "content": "  ORIGINAL\r\n"},
			map[string]any{"role": "assistant", "prefix": true, "content": "  PREFILL"},
		}, key: []any{map[string]any{"file_id": "fileA", "file_name": "UNIQUE_NAME.txt", "description": "UNIQUE_CONTEXT", "metadata": map[string]any{"business": "KEPT"}}}}
		before, _ := json.Marshal(raw)
		standard, err := NormalizeOpenAIChatRequest(nil, raw, "")
		if err != nil {
			t.Fatal(err)
		}
		bridge := standard.OpenAIChatRequest(raw)
		if bridge[key] != nil {
			t.Fatal("bridge copied already-normalized attachments twice")
		}
		rebuilt, err := NormalizeOpenAIChatRequest(nil, bridge, "")
		if err != nil {
			t.Fatal(err)
		}
		again, err := NormalizeOpenAIChatRequest(nil, raw, "")
		if err != nil || again.FinalPrompt != standard.FinalPrompt {
			t.Fatal("repeated normalization changed attachment context")
		}
		for _, text := range []string{standard.FinalPrompt, rebuilt.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
			for _, want := range []string{"UNIQUE_NAME.txt", "UNIQUE_CONTEXT", "KEPT", "  ORIGINAL\r\n"} {
				if strings.Count(text, want) != 1 {
					t.Fatalf("%s context missing/duplicated %q: %s", key, want, text)
				}
			}
		}
		if !strings.HasSuffix(rebuilt.FinalPrompt, "[Assistant]:  PREFILL") {
			t.Fatal("attachments displaced assistant prefill")
		}
		found := false
		for _, field := range standard.ContextManifest().InputFields {
			found = found || field.Name == key
		}
		if !found || len(standard.RefFileIDs) != 1 || len(standard.ContextManifest().Omissions) != 0 {
			t.Fatal("manifest lost attachment input or its representation")
		}
		after, _ := json.Marshal(raw)
		if string(before) != string(after) {
			t.Fatal("normalization mutated source messages or attachment blocks")
		}
	}
}

func TestPartialToolHistoryKeepsAttachmentContextBeforeItsLoop(t *testing.T) {
	raw := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c", "name": "read", "input": map[string]any{}}}},
		map[string]any{"role": "tool", "tool_call_id": "c", "content": "RESULT"},
	}, "files": []any{"fileA", map[string]any{"type": "file_reference", "id": "fileB", "displayName": "SECOND_FILE"}}}
	standard, err := NormalizeOpenAIChatRequest(nil, raw, "")
	if err != nil || !strings.Contains(standard.FinalPrompt, "SECOND_FILE") || len(standard.RefFileIDs) != 2 {
		t.Fatalf("partial history attachments lost: %+v %v", standard.RefFileIDs, err)
	}
	if strings.Index(standard.FinalPrompt, "fileA") > strings.Index(standard.FinalPrompt, `call_id="c"`) || strings.Index(standard.FinalPrompt, `call_id="c"`) > strings.Index(standard.FinalPrompt, "RESULT") {
		t.Fatal("attachment conversion changed the active tool loop order")
	}
}

func TestHistoryIdentityTypesAreValidatedBeforeNormalization(t *testing.T) {
	for _, value := range []any{123, true, []any{"c"}, map[string]any{"id": "c"}} {
		for _, kind := range []string{"function_call", "custom_tool_call", "tool_call", "function_call_output"} {
			item := map[string]any{"type": kind, "id": value, "name": "read", "arguments": "{}", "output": "RESULT"}
			if err := ValidateResponsesRequestContext(map[string]any{"input": []any{item}}); err == nil {
				t.Fatalf("native item ID disappeared before validation: %+v", item)
			}
		}
		for _, messages := range [][]any{
			{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": value, "name": "read"}}}},
			{map[string]any{"role": "tool", "tool_call_id": value, "content": "RESULT"}},
			{map[string]any{"role": "assistant", "function_call": map[string]any{"id": value, "name": "read"}}},
			{map[string]any{"role": "tool", "name": value, "content": "RESULT"}},
		} {
			for _, normalize := range []func() error{
				func() error {
					_, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, "")
					return err
				},
				func() error {
					_, err := NormalizeOpenAIResponsesRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}, "")
					return err
				},
			} {
				if normalize() == nil {
					t.Fatalf("invalid history identity accepted: %+v", messages)
				}
			}
		}
	}
	if err := ValidateToolHistory([]any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"name": "read"}}}, map[string]any{"role": "tool", "name": "read", "content": "RESULT"}}); err != nil {
		t.Fatalf("ID-less compatibility broken: %v", err)
	}
}
