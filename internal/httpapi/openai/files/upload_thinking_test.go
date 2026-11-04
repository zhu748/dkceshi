package files

import (
        "net/http/httptest"
        "net/url"
        "testing"
)

// 思考开关只由模型名决定：-nothinking 后缀关闭，无后缀开启。
// 表单 thinking_enabled 与 X-Thinking-Enabled 请求头仅兼容接收，不影响开关。
func TestUploadThinkingFollowsModelNameOnly(t *testing.T) {
        for _, tc := range []struct {
                model, form, header string
                want                bool
        }{
                {model: "deepseek-v4.1-flash", want: true},
                {model: "deepseek-v4.1-flash", form: "0", want: true},
                {model: "deepseek-v4.1-flash", header: "false", want: true},
                {model: "deepseek-v4.1-flash-nothinking", want: false},
                {model: "deepseek-v4.1-flash-nothinking", form: "1", want: false},
                {model: "deepseek-v4.1-flash-nothinking", header: "true", want: false},
                {model: "deepseek-v4.1-flash-search", want: true},
                {model: "deepseek-v4.1-flash-search-nothinking", want: false},
        } {
                r := httptest.NewRequest("POST", "/v1/files", nil)
                r.Form = url.Values{"model": {tc.model}, "thinking_enabled": {tc.form}}
                r.Header.Set("X-Thinking-Enabled", tc.header)
                if got := resolveUploadThinking(nil, r); got != tc.want {
                        t.Fatalf("%#v: got %v", tc, got)
                }
        }
}
