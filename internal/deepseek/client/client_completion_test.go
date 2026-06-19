package client

import (
        "context"
        "errors"
        "net/http"
        "testing"

        "ds2api/internal/auth"
)

func TestCallCompletionDoesNotFallbackForNonIdempotentCompletion(t *testing.T) {
        // 该测试通过 mock doer 模拟上游失败，必须关闭 framed H2 path（否则会绕过 mock 走真实网络）
        t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "0")

        var fallbackCalled bool
        client := &Client{
                stream: doerFunc(func(*http.Request) (*http.Response, error) {
                        return nil, errors.New("ambiguous completion write failure")
                }),
                fallbackS: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
                        fallbackCalled = true
                        return &http.Response{StatusCode: http.StatusOK}, nil
                })},
        }
        _, err := client.CallCompletion(
                context.Background(),
                &auth.RequestAuth{DeepSeekToken: "token"},
                map[string]any{"prompt": "hello"},
                "pow",
                3,
        )
        if err == nil {
                t.Fatal("expected completion error")
        }
        if fallbackCalled {
                t.Fatal("completion fallback should not be called for a non-idempotent request")
        }
}
