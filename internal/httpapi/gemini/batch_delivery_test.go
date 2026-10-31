package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func schemaLessGeminiRuntime() (*geminiStreamRuntime, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	return newGeminiStreamRuntime(rec, http.NewResponseController(rec), true, "model", "prompt", false, false, true, nil, nil, false, nil), rec
}

func feedGeminiText(rt *geminiStreamRuntime, text string) {
	rt.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: text}}})
}

func TestSchemaLessStreamWaitsForCompleteValidatedBatch(t *testing.T) {
	good := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	bad := `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`
	rt, rec := schemaLessGeminiRuntime()
	for _, chunk := range []string{good, "tail prose", bad} {
		feedGeminiText(rt, chunk)
		if rec.Body.Len() != 0 {
			t.Fatalf("unvalidated batch leaked: %s", rec.Body.String())
		}
	}
	if rt.finalize(true) || rt.finalErrorCode != "invalid_tool_arguments" {
		t.Fatalf("invalid batch accepted: %s", rec.Body.String())
	}
	rt.resetForRegeneration()
	feedGeminiText(rt, "corrected answer")
	if rec.Body.Len() == 0 {
		t.Fatal("ordinary prose stopped streaming")
	}
	if !rt.finalize(false) {
		t.Fatal("corrected answer was deferred")
	}
	if strings.Contains(rec.Body.String(), "invoke") || strings.Contains(rec.Body.String(), "tail prose") || strings.Count(rec.Body.String(), "corrected answer") != 1 {
		t.Fatalf("retry polluted or duplicated: %s", rec.Body.String())
	}
}

func TestSchemaLessStreamWithholdsSplitMarkersAndPreservesArguments(t *testing.T) {
	for _, size := range []int{1, 3, 17, 500} {
		rt, rec := schemaLessGeminiRuntime()
		rt.toolMarker, rt.toolSieve.Marker = "Q7ZK3M", "Q7ZK3M"
		text := `<|Q7ZK3M|tool_calls><|Q7ZK3M|invoke name="lookup"><|Q7ZK3M|parameter name="id">9007199254740993</|Q7ZK3M|parameter></|Q7ZK3M|invoke></|Q7ZK3M|tool_calls>`
		for start := 0; start < len(text); start += size {
			end := min(start+size, len(text))
			feedGeminiText(rt, text[start:end])
			if rec.Body.Len() != 0 {
				t.Fatalf("size=%d leaked candidate: %s", size, rec.Body.String())
			}
		}
		if !rt.finalize(false) {
			t.Fatal("valid batch deferred")
		}
		body := rec.Body.String()
		if strings.Count(body, `"functionCall"`) != 1 || !strings.Contains(body, "9007199254740993") || strings.Contains(body, "Q7ZK3M") {
			t.Fatalf("size=%d lost tool or precision: %s", size, body)
		}
	}
}

func TestGeminiStreamNeverRegeneratesAfterDeliveredText(t *testing.T) {
	rt, rec := schemaLessGeminiRuntime()
	feedGeminiText(rt, "already delivered")
	feedGeminiText(rt, `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`)
	if !rt.finalize(true) || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("delivered answer was discarded for regeneration: %s", rec.Body.String())
	}
}

func TestGeminiStreamDoesNotRetryPermanentUpstreamErrors(t *testing.T) {
	rt, rec := schemaLessGeminiRuntime()
	rt.upstreamErr = "input exceeds context length"
	if !rt.finalize(true) || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("permanent upstream error was deferred: %s", rec.Body.String())
	}
}
