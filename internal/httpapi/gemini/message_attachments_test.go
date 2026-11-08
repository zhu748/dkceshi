package gemini

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiMessageAttachmentsSurviveNativeFragmentSplitting(t *testing.T) {
	for _, key := range promptcompat.AttachmentInputKeys() {
		request := map[string]any{"contents": []any{
			map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": "c", "name": "read", "args": map[string]any{}}}}},
			map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"id": "c", "name": "read", "response": map[string]any{"result": "RESULT"}}}}, key: []any{
				map[string]any{"type": "input_file", "file_id": "file-a", "description": "GEMINI_ATTACHMENT"},
			}},
		}}
		standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", request, false)
		if err != nil {
			t.Fatal(err)
		}
		bridged, err := promptcompat.NormalizeOpenAIChatRequest(nil, standard.OpenAIChatRequest(request), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages), bridged.FinalPrompt} {
			if strings.Count(text, "GEMINI_ATTACHMENT") != 1 || !strings.Contains(text, "RESULT") || !strings.Contains(text, `call_id="c"`) {
				t.Fatal("native result splitting discarded attachment context or its tool chain")
			}
		}
	}
}

func TestGeminiToolPayloadAndPartAttachmentsRemainDistinct(t *testing.T) {
	request := map[string]any{"contents": []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": "c", "name": "read", "args": map[string]any{"file_data": "BUSINESS_BYTES"}, "files": []any{map[string]any{"file_id": "call-file", "description": "CALL_ATTACHMENT"}}}}}},
		map[string]any{"role": "user", "attachments": []any{map[string]any{"file_id": "message-file", "description": "MESSAGE_ATTACHMENT"}}, "parts": []any{map[string]any{
			"functionResponse": map[string]any{"id": "c", "name": "read", "response": map[string]any{"result": "RESULT"}, "attachments": []any{map[string]any{"file_id": "result-file", "description": "RESULT_ATTACHMENT"}}},
			"attachments":      []any{map[string]any{"file_id": "part-file", "description": "PART_ATTACHMENT"}},
		}}},
	}}
	standard, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", request, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{standard.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(standard.Messages)} {
		for _, want := range []string{"CALL_ATTACHMENT", "RESULT_ATTACHMENT", "PART_ATTACHMENT", "MESSAGE_ATTACHMENT", "BUSINESS_BYTES"} {
			if strings.Count(text, want) != 1 {
				t.Fatalf("native part context lost/duplicated %s", want)
			}
		}
	}
}
