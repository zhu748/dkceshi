package promptcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestImageRoutingPreservesBusinessObjects(t *testing.T) {
	for _, block := range []map[string]any{
		{"url": "https://example.com/report", "title": "BUSINESS_CONTEXT"},
		{"url": "data:image/png;base64,QUJD", "title": "BUSINESS_CONTEXT"},
		{"type": "business_result", "image_url": "thumbnail", "title": "BUSINESS_CONTEXT"},
		{"image_url": nil, "title": "BUSINESS_CONTEXT"},
	} {
		req := map[string]any{"model": "deepseek-v4.1-flash", "messages": []any{map[string]any{"role": "user", "content": []any{block}}}}
		before, _ := json.Marshal(req)
		route := PlanImageRoute(req, nil)
		if route.Rerouted {
			t.Fatalf("business data triggered image routing: %s", before)
		}
		route.StripImages(req)
		standard, err := NormalizeOpenAIChatRequest(nil, req, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
			if !strings.Contains(text, "BUSINESS_CONTEXT") {
				t.Fatal("business data disappeared from the prompt or history")
			}
		}
		after, _ := json.Marshal(req)
		if string(before) != string(after) {
			t.Fatal("image classification rewrote the source request")
		}
	}
}

func TestImageCleanupPreservesHistoricalToolResults(t *testing.T) {
	link := map[string]any{"url": "https://example.com/report", "title": "TOOL_RESULT_CONTEXT", "description": "  exact description\n"}
	image := map[string]any{"image_url": map[string]any{"url": "old locator"}, "file_id": "image-1", "description": "IMAGE_DESCRIPTION"}
	messages := []any{
		map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "c1", "name": "search", "arguments": map[string]any{}}}},
		map[string]any{"role": "tool", "tool_call_id": "c1", "content": []any{link}},
		map[string]any{"role": "user", "content": []any{image}},
	}
	req := map[string]any{"model": "deepseek-v4.1-flash", "messages": messages}
	before, _ := json.Marshal(messages)
	route := PlanImageRoute(req, nil)
	if !route.Rerouted {
		t.Fatal("explicit image_url did not trigger image handling")
	}
	route.StripImages(req)
	standard, err := NormalizeOpenAIChatRequest(nil, req, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{standard.FinalPrompt, BuildOpenAIHistoryTranscript(standard.Messages)} {
		for _, expected := range []string{"TOOL_RESULT_CONTEXT", "https://example.com/report", "IMAGE_DESCRIPTION", "image-1", "c1"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("image cleanup lost context %q", expected)
			}
		}
	}
	after, _ := json.Marshal(messages)
	if string(before) != string(after) || image["type"] != nil {
		t.Fatal("image cleanup mutated source messages or attachment")
	}
	if len(standard.RefFileIDs) != 1 || standard.RefFileIDs[0] != "image-1" {
		t.Fatalf("image reference lost: %v", standard.RefFileIDs)
	}
}
