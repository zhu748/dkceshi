package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestGeminiBlockAdapterPreservesOuterAssistantContext(t *testing.T) {
	for _, standalone := range []string{"  OUTER_REASON\r\n", "BLOCK_REASON"} {
		message := map[string]any{"role": "model", "reasoning_content": standalone, "refusal": "OUTER_REFUSAL", "audio": map[string]any{"id": "audio-ref", "transcript": "OUTER_TRANSCRIPT"}, "parts": []any{map[string]any{"text": "BODY"}, map[string]any{"text": "BLOCK_REASON", "thought": true}}}
		norm, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{"contents": []any{message}}, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{norm.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(norm.Messages)} {
			for _, want := range []string{"BLOCK_REASON", "OUTER_REFUSAL", "OUTER_TRANSCRIPT", "audio-ref"} {
				if strings.Count(text, want) != 1 {
					t.Fatalf("source=%q lost/duplicated %s: %s", standalone, want, text)
				}
			}
			if strings.Contains(standalone, "OUTER_REASON") && (!strings.Contains(text, standalone) || strings.Index(text, "OUTER_REASON") > strings.Index(text, "BODY")) {
				t.Fatal("standalone reasoning lost its leading position")
			}
		}
	}
}

func TestGeminiOuterContextIsValidatedBeforeFlattening(t *testing.T) {
	for _, extra := range []map[string]any{{"reasoning_content": 1}, {"refusal": 1}, {"audio": []any{}}, {"audio": map[string]any{"id": false}}} {
		message := map[string]any{"role": "model", "parts": []any{map[string]any{"text": "valid neighbor"}}}
		for key, value := range extra {
			message[key] = value
		}
		if _, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", map[string]any{"contents": []any{message}}, false); err == nil {
			t.Fatalf("invalid context disappeared: %+v", extra)
		}
	}
}

func TestGeminiJSONResultsRemainOpaqueAcrossChatBridge(t *testing.T) {
	for _, payload := range []any{
		[]any{map[string]any{"type": "text", "text": "JSON_TEXT"}, nil, true, json.Number("9007199254740993")},
		map[string]any{"type": "input_file", "file_id": "BUSINESS_ID", "file_data": "BUSINESS_DATA"},
		[]any{},
	} {
		for _, media := range []bool{false, true} {
			result := map[string]any{"id": "result", "name": "fetch", "response": payload}
			if media {
				result["parts"] = []any{map[string]any{"text": "REAL_PART"}}
			}
			req := map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": result}}}}}
			std, err := normalizeGeminiRequest(nil, "deepseek-v4.1-flash", req, false)
			if err != nil {
				t.Fatal(err)
			}
			bridge := std.OpenAIChatRequest(req)
			chat, err := promptcompat.NormalizeOpenAIChatRequest(nil, bridge, "")
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{std.FinalPrompt, chat.FinalPrompt, promptcompat.BuildOpenAIHistoryTranscript(chat.Messages)} {
				if !strings.Contains(text, stringifyJSON(payload)) || media && !strings.Contains(text, "REAL_PART") {
					t.Fatalf("JSON data was reinterpreted: %s", text)
				}
			}
			if len(promptcompat.CollectOpenAIRefFileIDs(bridge)) != 0 {
				t.Fatal("business file_id became an attachment")
			}
		}
	}
}
