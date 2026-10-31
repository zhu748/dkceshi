package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/promptcompat"
)

func TestVercelValidationUsesLeaseSchemaAndPreservesNumbers(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "validation-secret")
	tools := []any{map[string]any{"type": "function", "name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}, "required": []any{"id"}, "additionalProperties": false}}}
	h := &Handler{}
	lease := h.holdStreamLease(&auth.RequestAuth{CallerID: "owner"}, promptcompat.StandardRequest{ToolsRaw: tools, ToolNames: []string{"lookup"}, ToolChoice: promptcompat.ToolChoicePolicy{Mode: promptcompat.ToolChoiceAuto, RestrictAllowed: true, Allowed: map[string]struct{}{"lookup": {}}}}, "session")
	for _, tc := range []struct {
		name, content, token string
		status               int
	}{
		{"valid", `<tool_calls><invoke name="lookup"><parameter name="id">9007199254740993</parameter></invoke></tool_calls>`, "validation-secret", 200},
		{"missing_argument", `<tool_calls><invoke name="lookup"></invoke></tool_calls>`, "validation-secret", 422},
		{"undeclared", `<tool_calls><invoke name="other"></invoke></tool_calls>`, "validation-secret", 422},
		{"unauthorized", "hello", "wrong", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"lease_id": lease, "content": tc.content})
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
			req.Header.Set("X-Ds2-Internal-Token", tc.token)
			rec := httptest.NewRecorder()
			h.ChatCompletions(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("%d: %s", rec.Code, rec.Body.String())
			}
			if tc.status == 200 && !strings.Contains(rec.Body.String(), "9007199254740993") {
				t.Fatal("precision lost")
			}
			if tc.status == 422 && strings.Contains(rec.Body.String(), `"tool_calls"`) {
				t.Fatal("rejected tool escaped")
			}
		})
	}
}

func TestVercelSchemaLessFinalizationIsAtomicAndLossless(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("DS2API_VERCEL_INTERNAL_SECRET", "validation-secret")
	h := &Handler{}
	lease := h.holdStreamLease(&auth.RequestAuth{CallerID: "owner"}, promptcompat.StandardRequest{}, "session")
	good := `<tool_calls><invoke name="read"></invoke></tool_calls>`
	bad := `<tool_calls><invoke name="write"><param name="path">a.txt</param></invoke></tool_calls>`
	for _, tc := range []struct {
		name, content string
		status        int
	}{
		{"large integer", `<tool_calls><invoke name="lookup"><parameter name="id">9007199254740993</parameter></invoke></tool_calls>`, 200},
		{"malformed prefix", bad + good, 422},
		{"broken batch", `<tool_calls><invoke name="read"></invoke><invoke name="write"><parameter name="path">a.txt</parameter></tool_calls>`, 422},
		{"damaged optional argument", `<tool_calls><invoke name="write"><parameter name="path">a.txt</parameter><param name="content">important</param></invoke></tool_calls>`, 422},
		{"mixed DSML and damaged canonical", `<DSML calls><DSML invoke name="read"></DSML invoke></DSML calls>` + bad, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"lease_id": lease, "content": tc.content})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_validate=1", strings.NewReader(string(data)))
			req.Header.Set("X-Ds2-Internal-Token", "validation-secret")
			rec := httptest.NewRecorder()
			h.ChatCompletions(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.status == 200 && !strings.Contains(rec.Body.String(), "9007199254740993") {
				t.Fatal("argument precision lost")
			}
			if tc.status == 422 && strings.Contains(rec.Body.String(), `"tool_calls"`) {
				t.Fatal("partial tool batch escaped")
			}
		})
	}
}
