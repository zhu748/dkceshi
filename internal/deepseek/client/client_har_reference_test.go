package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	dsprotocol "ds2api/internal/deepseek/protocol"
	powpkg "ds2api/pow"
)

func TestPowHeaderMatchesCapturedBytesAndLifetime(t *testing.T) {
	var fixture struct {
		PowKeys  []string `json:"pow_keys"`
		Lifetime int64    `json:"challenge_expire_after_ms"`
	}
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	header, err := BuildPowHeader(map[string]any{"algorithm": "DeepSeekHashV1", "challenge": "test", "salt": "salt", "signature": "signature", "target_path": dsprotocol.DeepSeekUploadTargetPath}, 7)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"algorithm":"DeepSeekHashV1","challenge":"test","salt":"salt","signature":"signature","answer":7,"target_path":"/api/v0/file/upload_file"}`
	if string(decoded) != want {
		t.Fatalf("PoW wire JSON = %s", decoded)
	}
	d := json.NewDecoder(bytes.NewReader(decoded))
	if _, err := d.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for d.More() {
		key, err := d.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key.(string))
		var value any
		if err := d.Decode(&value); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(keys, fixture.PowKeys) {
		t.Fatalf("keys = %v", keys)
	}
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		challenge map[string]any
		want      time.Time
	}{
		{map[string]any{"expire_after": fixture.Lifetime}, now.Add(5 * time.Minute)},
		{map[string]any{"expire_after": int64(45000)}, now.Add(45 * time.Second)},
		{map[string]any{"expire_at": now.Add(time.Minute).UnixMilli()}, now.Add(time.Minute)},
		{map[string]any{"expire_at": now.Add(time.Minute).Unix()}, now.Add(time.Minute)},
		{map[string]any{}, time.Time{}},
	} {
		if got := powChallengeExpiry(tc.challenge, now); !got.Equal(tc.want) {
			t.Fatalf("expiry = %v, want %v", got, tc.want)
		}
	}
	cache := newPowPrefetchCache(nil)
	cache.now = func() time.Time { return now }
	cache.Put("account", "upload", "proof", now.Add(45*time.Second))
	cache.now = func() time.Time { return now.Add(25 * time.Second) }
	if _, ok := cache.Get("account", "upload"); ok {
		t.Fatal("used proof at APK 20-second expiration margin")
	}
	cache.Put("account", "upload", "proof", time.Time{})
	if cache.Size() != 0 {
		t.Fatal("cached a proof without a server lifetime")
	}
}

func TestDeleteRequestsRejectBusinessErrorsAndUseEmptyDeleteAll(t *testing.T) {
	for _, failed := range []bool{false, true} {
		a := &auth.RequestAuth{AccountID: "account", DeepSeekToken: "test-token"}
		client := &Client{maxRetries: 1}
		client.regular = alignmentDoer(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if req.URL.Path == "/api/v0/chat_session/delete_all" && (len(body) != 0 || req.ContentLength != 0 || req.Header.Get("Content-Type") != "") {
				t.Fatalf("delete_all body = %q", body)
			}
			if req.URL.Path == "/api/v0/chat_session/delete" && string(body) != `{"chat_session_id":"session-test","chat_session_ids":null}` {
				t.Fatalf("delete body = %s", body)
			}
			response := `{"code":0,"data":{"biz_code":0}}`
			if failed {
				response = `{"code":0,"data":{"biz_code":999,"biz_msg":"rejected"}}`
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
		})
		result, err := client.DeleteSessionForToken(context.Background(), a.DeepSeekToken, "session-test")
		if (err != nil) != failed || result.Success == failed {
			t.Fatalf("single deletion success=%v err=%v", result.Success, err)
		}
		if err := client.DeleteAllSessionsForToken(context.Background(), a.DeepSeekToken); (err != nil) != failed {
			t.Fatalf("all token deletion err=%v", err)
		}
		if err := client.DeleteAllSessions(context.Background(), a); (err != nil) != failed {
			t.Fatalf("all managed deletion err=%v", err)
		}
	}
}

func TestBatchFilePollingMatchesRepeatedQueryAndPendingStatuses(t *testing.T) {
	previous := fileReadySleep
	defer func() { fileReadySleep = previous }()
	fileReadySleep = func(context.Context, time.Duration) error { return nil }
	calls := 0
	client := &Client{}
	client.regular = alignmentDoer(func(req *http.Request) (*http.Response, error) {
		calls++
		want := []string{"file-a", "file-b"}
		if calls == 2 {
			want = []string{"file-b"}
		}
		if req.Method != "GET" || !reflect.DeepEqual(req.URL.Query()["file_ids"], want) {
			t.Fatalf("file query %s", req.URL)
		}
		if req.Header.Get("Content-Type") != "" || req.Header.Get("x-ds-pow-response") != "" {
			t.Fatal("unexpected file GET headers")
		}
		response := `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file-a","status":"SUCCESS"},{"id":"file-b","status":"PARSING"}]}}}`
		if calls == 2 {
			response = `{"code":0,"data":{"biz_code":0,"biz_data":{"files":[{"id":"file-b","status":"SUCCESS"}]}}}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	files := []*UploadFileResult{{ID: "file-a", Status: "PENDING"}, {ID: "file-b", Status: "PENDING"}}
	if err := client.WaitForUploadedFiles(context.Background(), &auth.RequestAuth{}, files); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || files[0].Status != "SUCCESS" || files[1].Status != "SUCCESS" {
		t.Fatalf("calls=%d files=%v", calls, files)
	}
	if err := client.WaitForUploadedFiles(context.Background(), &auth.RequestAuth{}, []*UploadFileResult{{ID: "file-a", Status: "FAILED"}}); !errors.Is(err, ErrUploadFileRejected) {
		t.Fatalf("failed file = %v", err)
	}
}

func TestMultipartEscapesSpecialFilenameWithoutChangingIt(t *testing.T) {
	body, _, err := buildUploadMultipartBody(`a"b\c.txt`, "text/plain", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`filename="a\"b\\c.txt"`)) {
		t.Fatalf("filename header = %s", body)
	}
	if got := appMultipartParameter(`"already quoted.txt"`); got != `"already quoted.txt"` {
		t.Fatalf("Ktor quoted value changed: %s", got)
	}
}

func TestDeletionDoesNotAcceptMissingResultCodes(t *testing.T) {
	for _, response := range []string{`{}`, `{"code":0,"data":{}}`, `{"code":0}`, `not-json`} {
		client := &Client{regular: alignmentDoer(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
		})}
		if _, err := client.DeleteSessionForToken(context.Background(), "test-token", "session-test"); err == nil {
			t.Fatalf("accepted invalid single-delete response: %s", response)
		}
		if err := client.DeleteAllSessionsForToken(context.Background(), "test-token"); err == nil {
			t.Fatalf("accepted invalid delete-all response: %s", response)
		}
	}
}

func TestPowUsesRequestTargetWhenResponseOmitsOptionalField(t *testing.T) {
	hash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	challenge := map[string]any{"algorithm": "DeepSeekHashV1", "challenge": hex.EncodeToString(hash[:]), "salt": "salt", "signature": "test-signature", "expire_at": int64(1712345678), "expire_after": int64(300000), "difficulty": int64(1000)}
	response, err := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"biz_code": 0, "biz_data": map[string]any{"challenge": challenge}}})
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{regular: alignmentDoer(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(response))}, nil
	})}
	now := time.Now()
	solution, err := client.fetchPowSolution(context.Background(), &auth.RequestAuth{}, dsprotocol.DeepSeekUploadTargetPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(solution.header)
	if err != nil {
		t.Fatal(err)
	}
	var proof map[string]any
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	if proof["target_path"] != dsprotocol.DeepSeekUploadTargetPath || solution.expiresAt.Before(now.Add(4*time.Minute)) {
		t.Fatalf("target or lifetime lost: %s", raw)
	}
}
