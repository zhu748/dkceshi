package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAudioContextSurvivesPromptHistoryAndReportsBinaryOmissions(t *testing.T) {
	audio := map[string]any{"id": "audio1", "transcript": "  EXACT_TRANSCRIPT\r\n", "expires_at": json.Number("9007199254740993"), "metadata": map[string]any{"speaker": "SPEAKER_UNIQUE", "data": "BUSINESS_DATA"}, "data": "BINARY_AUDIO_BYTES", "signature": "OPAQUE_AUDIO_SIGNATURE"}
	message := map[string]any{"role": "assistant", "content": "hello", "audio": audio}
	request, err := NormalizeOpenAIChatRequest(nil, map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{message}}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{request.FinalPrompt, BuildOpenAIHistoryTranscript(request.Messages)} {
		for _, want := range []string{"  EXACT_TRANSCRIPT\r\n", "audio1", "SPEAKER_UNIQUE", "BUSINESS_DATA", "9007199254740993"} {
			if !strings.Contains(text, want) {
				t.Fatalf("lost %q", want)
			}
		}
		if strings.Contains(text, "BINARY_AUDIO_BYTES") || strings.Contains(text, "OPAQUE_AUDIO_SIGNATURE") {
			t.Fatal("audio bytes leaked into chat text")
		}
	}
	manifest := request.ContextManifest()
	for _, want := range []string{"/audio/data", "/audio/signature"} {
		found := false
		for _, omission := range manifest.Omissions {
			found = found || omission.Field == want
		}
		if !found {
			t.Fatalf("missing omission %s", want)
		}
	}
	if audio["data"] != "BINARY_AUDIO_BYTES" {
		t.Fatal("caller audio object changed")
	}
}

func TestAudioContextRetainsExplicitNullAndEmptyReferences(t *testing.T) {
	for _, id := range []any{nil, ""} {
		encoded, _ := json.Marshal(map[string]any{"id": id, "transcript": nil})
		text := RenderPromptText([]any{map[string]any{"role": "assistant", "audio": map[string]any{"id": id, "transcript": nil}}}, false)
		if !strings.Contains(text, string(encoded)) {
			t.Fatal("explicit null/empty audio context was dropped")
		}
	}
}
