package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestFileTokenUsagePreservesKnownZeroAndRejectsInvalidCounts(t *testing.T) {
	for _, raw := range []any{0, int64(0), float64(0), json.Number("0"), "0", 954, json.Number("954")} {
		result := extractUploadFileResult(map[string]any{"data": map[string]any{"biz_data": map[string]any{"id": "file-test", "token_usage": raw}}})
		want, _ := nonNegativeFileTokens(raw)
		if result.TokenUsage == nil || *result.TokenUsage != want {
			t.Fatalf("raw=%v result=%+v", raw, result)
		}
		fetched := buildUploadFileResultFromMap(map[string]any{"id": "file-test", "token_usage": raw}, "file-test")
		if fetched.TokenUsage == nil || *fetched.TokenUsage != want {
			t.Fatalf("fetch lost token count: %+v", fetched)
		}
	}
	for _, raw := range []any{nil, -1, -1.0, 1.5, math.NaN(), math.Inf(1), 1e30, "-1", "1.5", "invalid", true} {
		if _, ok := nonNegativeFileTokens(raw); ok {
			t.Fatalf("accepted invalid count %#v", raw)
		}
	}
}

func TestFileTokenUsageSurvivesCacheCopiesAndUnknownUpdates(t *testing.T) {
	tokens := 954
	file := &UploadFileResult{ID: "file-test", TokenUsage: &tokens, Raw: map[string]any{"secret": true}}
	copy := uploadMetadataCopy(file)
	*copy.TokenUsage = 123
	if *file.TokenUsage != 954 || copy.Raw != nil {
		t.Fatal("cache metadata shares mutable state")
	}
	mergeUploadFileResults(file, &UploadFileResult{ID: file.ID, Status: "SUCCESS"})
	if *file.TokenUsage != 954 {
		t.Fatal("missing metadata erased a known count")
	}
	zero := 0
	mergeUploadFileResults(file, &UploadFileResult{ID: file.ID, TokenUsage: &zero})
	zero = 22
	if *file.TokenUsage != 0 {
		t.Fatal("known zero lost or aliased")
	}
}

func TestFetchUploadedFilesRejectsPartialResults(t *testing.T) {
	c, _, a, _ := newReuseTestClient()
	c.regular = doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file-a","status":"SUCCESS","token_usage":954}]}}}`))}, nil
	})
	_, err := c.FetchUploadedFiles(context.Background(), a, []string{"file-a", "file-b"})
	if !errors.Is(err, ErrUploadFileNotFound) || !isPermanentFileFetchError(err) {
		t.Fatalf("partial file response was accepted or retried: %v", err)
	}
}

func TestFetchUploadedFilesPreservesHTTPFailureStatus(t *testing.T) {
	for _, status := range []int{200, 429, 503} {
		c, _, a, _ := newReuseTestClient()
		c.regular = doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"code":9,"data":{"biz_code":9}}`))}, nil
		})
		_, err := c.FetchUploadedFiles(context.Background(), a, []string{"file-test"})
		var failure interface{ HTTPStatus() int }
		want := status
		if status == 200 {
			want = 502
		}
		if !errors.As(err, &failure) || failure.HTTPStatus() != want {
			t.Fatalf("status=%d err=%v", status, err)
		}
	}
}
