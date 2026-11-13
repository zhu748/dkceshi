package ollama

import (
	"ds2api/internal/httpapi/requestbody"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelQueryRejectsOversizedBodiesForEveryContentType(t *testing.T) {
	for _, mime := range []string{"", "text/plain", "application/json"} {
		t.Run(mime, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/show", strings.NewReader(`{"model":"`+strings.Repeat("x", 64<<10)+`"}`))
			req.Header.Set("Content-Type", mime)
			rec := httptest.NewRecorder()
			requestbody.ValidateJSONUTF8(http.HandlerFunc((&Handler{}).GetOllamaModel)).ServeHTTP(rec, req)
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
