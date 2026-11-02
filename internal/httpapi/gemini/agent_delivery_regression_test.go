package gemini

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/sse"
)

func TestRecoveredDSMLCannotEmitTail(t *testing.T) {
	raw := `before <｜｜DSML｜｜ calls><｜｜DSML｜｜ invoke name="read"></｜｜DSML｜｜ invoke></｜｜DSML｜｜ calls> FAKE_RESULT`
	for _, names := range [][]string{nil, {"read"}} {
		for _, size := range []int{1, len(raw)} {
			rec := httptest.NewRecorder()
			s := newGeminiStreamRuntime(rec, http.NewResponseController(rec), true, "deepseek-v4.1-flash", "prompt", false, false, false, names, nil, false, nil)
			for i := 0; i < len(raw); i += size {
				s.onParsed(sse.LineResult{Parsed: true, Parts: []sse.ContentPart{{Type: "text", Text: raw[i:min(i+size, len(raw))]}}})
			}
			s.finalize(false)
			if strings.Contains(rec.Body.String(), "FAKE_RESULT") || !strings.Contains(rec.Body.String(), `"functionCall"`) {
				t.Fatalf("names=%v size=%d: %s", names, size, rec.Body.String())
			}
		}
	}
}
