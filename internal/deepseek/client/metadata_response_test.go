package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestMetadataResponseRejectsMalformedSuccessAndKeepsHTTPFailure(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		for _, status := range []int{200, 401, 404, 429, 503} {
			for _, body := range []string{"", "null", "[]", "<html>error</html>", `{"partial":1,`} {
				closed := false
				doer := doerFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: &metadataTestBody{Reader: strings.NewReader(body), closed: &closed}}, nil
				})
				c := &Client{regular: doer}
				var gotStatus int
				var err error
				if method == "GET" {
					_, gotStatus, err = c.getJSONWithStatus(context.Background(), doer, "https://example.test/metadata", nil)
				} else {
					_, gotStatus, err = c.postJSONWithStatus(context.Background(), doer, nil, "https://example.test/metadata", nil, nil)
				}
				if gotStatus != status || !closed || (status == 200) != errors.Is(err, ErrInvalidUpstreamResponse) {
					t.Fatalf("%s status=%d body=%q gotStatus=%d closed=%v err=%v", method, status, body, gotStatus, closed, err)
				}
			}
		}
	}
}

type metadataTestBody struct {
	io.Reader
	closed *bool
}

func (b *metadataTestBody) Close() error { *b.closed = true; return nil }

func TestMetadataSizeLimitAppliesAfterDecompression(t *testing.T) {
	data := bytes.Repeat([]byte("x"), maxMetadataResponseBytes+1)
	for _, encoding := range []string{"", "gzip", "br"} {
		var encoded bytes.Buffer
		var writer io.WriteCloser
		switch encoding {
		case "gzip":
			writer = gzip.NewWriter(&encoded)
		case "br":
			writer = brotli.NewWriterLevel(&encoded, 1)
		}
		body := data
		if writer != nil {
			if _, err := writer.Write(data); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			body = encoded.Bytes()
		}
		resp := &http.Response{Header: http.Header{"Content-Encoding": {encoding}}, Body: io.NopCloser(bytes.NewReader(body))}
		if _, err := readResponseBody(resp); !errors.Is(err, ErrUpstreamResponseTooLarge) {
			t.Fatalf("encoding=%q: %v", encoding, err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
