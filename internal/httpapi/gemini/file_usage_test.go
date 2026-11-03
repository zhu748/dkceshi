package gemini

import (
	"encoding/json"
	"net/http/httptest"
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
