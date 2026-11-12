package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiUsageIncludesActualFileTokens(t *testing.T) {
	count := func(tokens int) int {
		store, resolver := newGeminiMarkerResolver(t)
		ds := &geminiPhotoDS{geminiMarkerDS: geminiMarkerDS{toolCall: "ok"}, tokens: &tokens}
		h := &Handler{Store: store, Auth: resolver, DS: ds}
		req := geminiMarkerRequest(t, `{"contents":[{"role":"user","parts":[{"text":"describe"},{"inlineData":{"mimeType":"image/png","data":"QUJDRA=="}}]}]}`)
		rec := httptest.NewRecorder()
		h.GenerateContent(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		var result struct {
			Usage struct {
				Prompt int `json:"promptTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Usage.Prompt
	}
	if delta := count(954) - count(0); delta != 954 {
		t.Fatalf("file usage delta=%d", delta)
	}
}

func TestGeminiStreamingIncludesFileUsage(t *testing.T) {
	count := func(tokens int) int {
		store, resolver := newGeminiMarkerResolver(t)
		ds := &geminiPhotoDS{geminiMarkerDS: geminiMarkerDS{toolCall: "ok"}, tokens: &tokens}
		h := &Handler{Store: store, Auth: resolver, DS: ds}
		req := geminiMarkerRequest(t, `{"contents":[{"role":"user","parts":[{"text":"describe"},{"inlineData":{"mimeType":"image/png","data":"QUJDRA=="}}]}]}`)
		rec := httptest.NewRecorder()
		h.StreamGenerateContent(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		final := -1
		for _, line := range strings.Split(rec.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var frame map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
				t.Fatal(err)
			}
			if usage, ok := frame["usageMetadata"].(map[string]any); ok {
				final = int(usage["promptTokenCount"].(float64))
			}
		}
		if final < 0 {
			t.Fatalf("missing final usage: %s", rec.Body)
		}
		return final
	}
	if delta := count(954) - count(0); delta != 954 {
		t.Fatalf("file token delta=%d", delta)
	}
}
