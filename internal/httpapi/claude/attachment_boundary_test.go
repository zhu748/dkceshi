package claude

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestClaudeOutputConfigRejectsMalformedContainers(t *testing.T) {
	for _, value := range []any{false, "json", []any{}, 123} {
		_, err := normalizeClaudeRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": "JSON"}}, "output_config": value})
		if err == nil || !strings.Contains(err.Error(), "output_config") {
			t.Fatalf("malformed config silently disabled output validation: %T", value)
		}
	}
}

func TestClaudeMessageAttachmentsSurviveNativeFragmentSplitting(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "c", "name": "read", "input": map[string]any{}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c", "content": "RESULT"}}, key: []any{
				map[string]any{"type": "input_file", "file_id": "file-a", "description": "CLAUDE_ATTACHMENT"},
			}},
		}}
		normalized, err := normalizeClaudeRequest(nil, request)
		if err != nil {
			t.Fatal(err)
		}
		bridged, err := promptcompat.NormalizeOpenAIChatRequest(nil, normalized.Standard.OpenAIChatRequest(request), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{normalized.Standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(normalized.Standard.Messages), bridged.FinalPrompt} {
			if strings.Count(text, "CLAUDE_ATTACHMENT") != 1 || !strings.Contains(text, "RESULT") || !strings.Contains(text, `call_id="c"`) {
				t.Fatal("native result splitting discarded attachment context or its tool chain")
			}
		}
	}
}

func TestClaudeNativeToolItemsKeepTheirOwnAttachments(t *testing.T) {
	request := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{
		map[string]any{"role": "assistant", "attachments": []any{map[string]any{"file_id": "source-call-file", "description": "MESSAGE_CALL_ATTACHMENT"}}, "content": []any{map[string]any{"type": "tool_use", "id": "c", "name": "read", "input": map[string]any{"file_data": "BUSINESS_BYTES"}, "attachments": []any{map[string]any{"file_id": "call-file", "description": "ITEM_CALL_ATTACHMENT"}}}}},
		map[string]any{"role": "user", "files": []any{map[string]any{"file_id": "source-result-file", "description": "MESSAGE_RESULT_ATTACHMENT"}}, "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "c", "content": "RESULT", "files": []any{map[string]any{"file_id": "result-file", "description": "ITEM_RESULT_ATTACHMENT"}}}}},
	}}
	normalized, err := normalizeClaudeRequest(nil, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{normalized.Standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(normalized.Standard.Messages)} {
		for _, want := range []string{"MESSAGE_CALL_ATTACHMENT", "MESSAGE_RESULT_ATTACHMENT", "ITEM_CALL_ATTACHMENT", "ITEM_RESULT_ATTACHMENT", "BUSINESS_BYTES"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("native tool item context lost/duplicated %s", want)
			}
		}
	}
}
