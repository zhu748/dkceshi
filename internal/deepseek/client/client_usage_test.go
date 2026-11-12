package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ds2api/internal/sse"
)

func TestMessageUsageUsesOwningAccountAndCachesFinalCounts(t *testing.T) {
	c, _, a, _ := newReuseTestClient()
	calls := 0
	c.regular = doerFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v0/chat/history_messages" || r.URL.Query().Get("scenario") != "stream_close" || r.URL.Query().Get("chat_session_id") != "session" {
			t.Fatalf("wrong usage lookup: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer credential" {
			t.Fatal("changed account identity")
		}
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > messageUsageTimeout {
			t.Fatal("lookup is unbounded")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"biz_code":0,"biz_data":{"chat_session":{"id":"session"},"chat_messages":[{"message_id":1,"parent_id":null,"role":"USER","status":"FINISHED","accumulated_token_usage":58},{"message_id":2,"parent_id":1,"role":"ASSISTANT","status":"FINISHED","accumulated_token_usage":81}]}}}`))}, nil
	})
	resolver := c.NewUsageResolver(a, "session")
	for range 2 {
		u, err := resolver.ResolveUpstreamUsage(context.Background(), 2)
		if err != nil || *u.InputTokens != 58 || *u.OutputTokens != 23 || *u.TotalTokens != 81 {
			t.Fatalf("wrong usage %+v %v", u, err)
		}
	}
	if calls != 1 {
		t.Fatalf("preview/release queried %d times", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveUpstreamUsage(ctx, 2); err == nil {
		t.Fatal("ignored cancellation")
	}
	body := &usageResponseBody{ReadCloser: io.NopCloser(strings.NewReader("data: {\"response_message_id\":2,\"request_message_id\":1}\n\ndata: {\"p\":\"response/content\",\"v\":\"answer\"}\n\ndata: {\"p\":\"response/status\",\"v\":\"FINISHED\"}\n\ndata: [DONE]\n\n")), Resolver: resolver}
	result := sse.CollectStream(&http.Response{Body: body}, false, true)
	if result.ReadError != nil || result.UpstreamUsage.Request == nil || *result.UpstreamUsage.Request.OutputTokens != 23 {
		t.Fatalf("collector did not use authoritative counts: %+v", result)
	}
	body = &usageResponseBody{ReadCloser: io.NopCloser(strings.NewReader("data: {\"response_message_id\":2}\n\ndata: {\"p\":\"response/content\",\"v\":\"answer\"}\n\ndata: {\"p\":\"response/status\",\"v\":\"FINISHED\"}\n\ndata: [DONE]\n\n")), Resolver: resolver}
	lines, done := sse.StartParsedLinePump(context.Background(), body, false, "text")
	resolved := false
	for line := range lines {
		if line.ResolvedUsage {
			resolved = true
			if *line.UpstreamUsage.OutputTokens != 23 {
				t.Fatal("wrong streaming output count")
			}
		}
		if line.Stop && !resolved {
			t.Fatal("terminal frame preceded upstream usage")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !resolved || calls != 1 {
		t.Fatalf("streaming counts missing or repeated query: resolved=%v calls=%d", resolved, calls)
	}
}

func TestUsageLookupFailureKeepsSuccessfulOutputAndCachesFallback(t *testing.T) {
	c, _, a, _ := newReuseTestClient()
	queries := 0
	c.regular = doerFunc(func(*http.Request) (*http.Response, error) {
		queries++
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":1}`))}, nil
	})
	resolver := c.NewUsageResolver(a, "session")
	body := &usageResponseBody{ReadCloser: io.NopCloser(strings.NewReader("data: {\"response_message_id\":2}\n\ndata: {\"p\":\"response/content\",\"v\":\"answer\"}\n\ndata: [DONE]\n\n")), Resolver: resolver}
	result := sse.CollectStream(&http.Response{Body: body}, false, true)
	if result.ReadError != nil || result.Text != "answer" || result.UpstreamUsage.Request != nil {
		t.Fatalf("usage lookup broke successful generation: %+v", result)
	}
	if _, err := resolver.ResolveUpstreamUsage(context.Background(), 2); err == nil || queries != 1 {
		t.Fatalf("fallback was not cached: queries=%d err=%v", queries, err)
	}
}
