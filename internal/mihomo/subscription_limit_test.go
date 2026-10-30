package mihomo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type subscriptionRoundTripper func(*http.Request) (*http.Response, error)

func (f subscriptionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchSubscriptionRejectsTruncationAtSizeLimit(t *testing.T) {
	full := clashSubscriptionYAML + strings.Repeat(" ", subscriptionMaxBytes-len(clashSubscriptionYAML))
	previous := subscriptionHTTPClient
	t.Cleanup(func() { subscriptionHTTPClient = previous })
	for _, extra := range []string{"", "\n# beyond the limit"} {
		subscriptionHTTPClient = &http.Client{Transport: subscriptionRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, ContentLength: -1,
				Body: io.NopCloser(strings.NewReader(full + extra))}, nil
		})}
		nodes, err := FetchSubscription(context.Background(), "https://subscription.example/test")
		if extra == "" {
			if err != nil || len(nodes) != 2 {
				t.Fatalf("exact limit should remain usable: nodes=%d error=%v", len(nodes), err)
			}
		} else if !errors.Is(err, errContentTooLarge) || len(nodes) != 0 {
			t.Fatalf("oversize subscription silently accepted a truncated prefix: nodes=%d error=%v", len(nodes), err)
		}
	}
}
