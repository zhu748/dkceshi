package files

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestExplicitUploadThinkingMatchesModelAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		model, form, header string
		want                bool
	}{
		{model: "deepseek-v4.1-flash", want: true},
		{model: "deepseek-v4.1-flash", form: "0", want: false},
		{model: "deepseek-v4.1-flash", header: "false", want: false},
		{model: "deepseek-v4.1-flash-nothinking", form: "1", want: false},
	} {
		r := httptest.NewRequest("POST", "/v1/files", nil)
		r.Form = url.Values{"model": {tc.model}, "thinking_enabled": {tc.form}}
		r.Header.Set("X-Thinking-Enabled", tc.header)
		if got := resolveUploadThinking(nil, r); got != tc.want {
			t.Fatalf("%#v: got %v", tc, got)
		}
	}
}
