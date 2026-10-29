package client

import (
	"bytes"
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/image/webp"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"ds2api/internal/auth"
	powpkg "ds2api/pow"
)

func TestBuildUploadMultipartBodyOmitsPurposeAndIncludesFilePart(t *testing.T) {
	body, contentType, err := buildUploadMultipartBody(`../demo.txt`, "text/plain", []byte("hello"))
	if err != nil {
		t.Fatalf("buildUploadMultipartBody error: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	payload := string(body)
	if strings.Contains(payload, `name="purpose"`) || strings.Contains(payload, "assistants") {
		t.Fatalf("expected purpose to be omitted from payload: %q", payload)
	}
	if !strings.Contains(payload, `name=file; filename=demo.txt`) {
		t.Fatalf("expected sanitized filename in payload: %q", payload)
	}
	if !strings.Contains(payload, "Content-Type: application/octet-stream") {
		t.Fatalf("expected file content type in payload: %q", payload)
	}
	if !strings.Contains(payload, "hello") {
		t.Fatalf("expected file content in payload: %q", payload)
	}
}

func TestDoUploadFallsBackWhenPrimaryTransportFails(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "0")
	var fallbackCalled bool
	client := &Client{}
	_, err := client.doUpload(
		context.Background(),
		doerFunc(func(req *http.Request) (*http.Response, error) {
			_, _ = io.ReadAll(req.Body)
			return nil, errors.New("ambiguous upload write failure")
		}),
		doerFunc(func(*http.Request) (*http.Response, error) {
			fallbackCalled = true
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}),
		dsprotocol.DeepSeekUploadFileURL,
		map[string]string{"Content-Type": "multipart/form-data"},
		func() io.Reader { return strings.NewReader("body") },
	)
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}
	if !fallbackCalled {
		t.Fatal("expected upload fallback to be invoked on transport failure")
	}
}

func TestExtractUploadFileResultSupportsNestedShapes(t *testing.T) {
	got := extractUploadFileResult(map[string]any{
		"data": map[string]any{
			"biz_data": map[string]any{
				"file": map[string]any{
					"file_id":   "file_123",
					"file_name": "report.pdf",
					"file_size": 99,
					"status":    "processed",
					"purpose":   "assistants",
					"is_image":  true,
				},
			},
		},
	})
	if got.ID != "file_123" {
		t.Fatalf("expected id file_123, got %#v", got)
	}
	if got.Filename != "report.pdf" {
		t.Fatalf("expected filename report.pdf, got %#v", got)
	}
	if got.Bytes != 99 {
		t.Fatalf("expected bytes 99, got %#v", got)
	}
	if got.Status != "processed" {
		t.Fatalf("expected status processed, got %#v", got)
	}
	if got.Purpose != "assistants" {
		t.Fatalf("expected purpose assistants, got %#v", got)
	}
	if !got.IsImage {
		t.Fatalf("expected image flag true, got %#v", got)
	}
}

func TestUploadFileUsesUploadTargetPowAndMultipartHeaders(t *testing.T) {
	// Requests remain on the injected account transport, including ordered HTTP/2.
	t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "0")

	challengeHash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	powResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"challenge":{"algorithm":"DeepSeekHashV1","challenge":"` + hex.EncodeToString(challengeHash[:]) + `","salt":"salt","expire_at":1712345678,"difficulty":1000,"signature":"sig","target_path":"` + dsprotocol.DeepSeekUploadTargetPath + `"}}}}`
	uploadResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"file":{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"processed","purpose":"assistants","is_image":false}}}}`
	var seenPow string
	var seenTargetPath string
	var seenContentType string
	var seenFileSize string
	var seenModelType string
	var seenBody string
	call := 0
	client := &Client{
		regular: doerFunc(func(req *http.Request) (*http.Response, error) {
			call++
			bodyBytes, _ := io.ReadAll(req.Body)
			switch call {
			case 1:
				seenTargetPath = string(bodyBytes)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(powResponse)), Request: req}, nil
			case 2:
				seenPow = req.Header.Get("x-ds-pow-response")
				seenContentType = req.Header.Get("Content-Type")
				seenFileSize = req.Header.Get("x-file-size")
				seenModelType = req.Header.Get("x-model-type")
				seenBody = string(bodyBytes)
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(uploadResponse)), Request: req}, nil
			default:
				t.Fatalf("unexpected request count %d", call)
				return nil, nil
			}
		}),
		fallback: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, nil
		})},
		maxRetries: 1,
	}
	result, err := client.UploadFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token", TriedAccounts: map[string]bool{}}, UploadFileRequest{
		Filename:    "demo.txt",
		ContentType: "text/plain",
		Purpose:     "assistants",
		ModelType:   "vision",
		Data:        []byte("hello"),
	}, 1)
	if err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}
	if result.ID != "file_789" {
		t.Fatalf("expected uploaded file id file_789, got %#v", result)
	}
	if !strings.Contains(seenTargetPath, `"target_path":"`+dsprotocol.DeepSeekUploadTargetPath+`"`) {
		t.Fatalf("expected upload target_path in pow request, got %q", seenTargetPath)
	}
	if strings.TrimSpace(seenPow) == "" {
		t.Fatal("expected x-ds-pow-response header")
	}
	rawPow, err := base64.StdEncoding.DecodeString(seenPow)
	if err != nil {
		t.Fatalf("decode pow header failed: %v", err)
	}
	var powHeader map[string]any
	if err := json.Unmarshal(rawPow, &powHeader); err != nil {
		t.Fatalf("unmarshal pow header failed: %v", err)
	}
	if powHeader["target_path"] != dsprotocol.DeepSeekUploadTargetPath {
		t.Fatalf("expected pow target_path %q, got %#v", dsprotocol.DeepSeekUploadTargetPath, powHeader["target_path"])
	}
	if seenFileSize != "5" {
		t.Fatalf("expected x-file-size=5, got %q", seenFileSize)
	}
	if seenModelType != "default" {
		t.Fatalf("expected x-model-type=default, got %q", seenModelType)
	}
	if !strings.HasPrefix(seenContentType, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content type, got %q", seenContentType)
	}
	if !strings.Contains(seenBody, `name=file; filename=demo.txt`) {
		t.Fatalf("expected file part in upload body: %q", seenBody)
	}
}

func TestUploadFileWaitsForProcessedFetchFiles(t *testing.T) {
	// Requests remain on the injected account transport, including ordered HTTP/2.
	t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "0")

	oldSleep := fileReadySleep
	fileReadySleep = func(context.Context, time.Duration) error { return nil }
	defer func() { fileReadySleep = oldSleep }()

	challengeHash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	powResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"challenge":{"algorithm":"DeepSeekHashV1","challenge":"` + hex.EncodeToString(challengeHash[:]) + `","salt":"salt","expire_at":1712345678,"difficulty":1000,"signature":"sig","target_path":"` + dsprotocol.DeepSeekUploadTargetPath + `"}}}}`
	uploadResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"file":{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"PENDING","purpose":"assistants","is_image":false}}}}`
	pendingFetchResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"files":[{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"PENDING","purpose":"assistants","is_image":false}]}}}`
	processedFetchResponse := `{"code":0,"msg":"ok","data":{"biz_code":0,"biz_data":{"files":[{"file_id":"file_789","filename":"demo.txt","bytes":5,"status":"processed","purpose":"assistants","is_image":true}]}}}`

	var call int
	client := &Client{
		regular: doerFunc(func(req *http.Request) (*http.Response, error) {
			call++
			switch call {
			case 1:
				bodyBytes, _ := io.ReadAll(req.Body)
				if !strings.Contains(string(bodyBytes), `"target_path":"`+dsprotocol.DeepSeekUploadTargetPath+`"`) {
					t.Fatalf("expected pow target path request, got %s", string(bodyBytes))
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(powResponse)), Request: req}, nil
			case 2:
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(uploadResponse)), Request: req}, nil
			case 3, 4:
				if req.Method != http.MethodGet {
					t.Fatalf("expected GET fetch request, got %s", req.Method)
				}
				if req.URL.Path != "/api/v0/file/fetch_files" {
					t.Fatalf("expected fetch files path /api/v0/file/fetch_files, got %q", req.URL.Path)
				}
				if got := req.URL.Query().Get("file_ids"); got != "file_789" {
					t.Fatalf("expected file_ids=file_789, got %q", got)
				}
				respBody := pendingFetchResponse
				if call == 4 {
					respBody = processedFetchResponse
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(respBody)), Request: req}, nil
			default:
				t.Fatalf("unexpected request count %d", call)
				return nil, nil
			}
		}),
		fallback:   &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) { return nil, nil })},
		maxRetries: 1,
	}

	result, err := client.UploadFile(context.Background(), &auth.RequestAuth{DeepSeekToken: "token", TriedAccounts: map[string]bool{}}, UploadFileRequest{
		Filename:    "demo.txt",
		ContentType: "text/plain",
		Purpose:     "assistants",
		Data:        []byte("hello"),
	}, 1)
	if err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}
	if result.ID != "file_789" {
		t.Fatalf("expected uploaded file id file_789, got %#v", result)
	}
	if result.Status != "processed" {
		t.Fatalf("expected final status processed, got %#v", result.Status)
	}
	if call != 4 {
		t.Fatalf("expected 4 requests, got %d", call)
	}
}

func TestPhotoUploadHeadersDescribeTranscodedBytes(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_PHOTO_COMPRESS_RATIO", "0.8")
	var input bytes.Buffer
	if err := png.Encode(&input, uploadTestImage()); err != nil {
		t.Fatal(err)
	}
	client := &Client{maxRetries: 1, powCache: newPowPrefetchCache(nil)}
	client.powCache.Put("account", dsprotocol.DeepSeekUploadTargetPath, "test-pow", time.Now().Add(time.Hour))
	client.regular = doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v0/file/upload_file" {
			t.Fatalf("wrong path: %s", req.URL.Path)
		}
		if req.Header.Get("x-thinking-enabled") != "1" || req.Header.Get("x-model-type") != "default" {
			t.Fatal("thinking/model upload headers mismatch")
		}
		if req.ContentLength > 0 || req.Header.Get("Content-Length") != "" {
			t.Fatal("multipart must have unknown total length")
		}
		mime, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if err != nil || mime != "multipart/form-data" || len(params["boundary"]) != 70 {
			t.Fatal("multipart boundary mismatch")
		}
		reader := multipart.NewReader(req.Body, params["boundary"])
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() != "file" || !strings.HasPrefix(part.FileName(), "photo_") || !strings.HasSuffix(part.FileName(), ".webp") || part.Header.Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("part headers mismatch: %v", part.Header)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if err := part.Close(); err != nil {
			t.Fatal(err)
		}
		if req.Header.Get("x-file-size") != strconv.Itoa(len(data)) {
			t.Fatal("file size does not match transcoded data")
		}
		if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.NextPart(); err != io.EOF {
			t.Fatalf("unexpected second part: %v", err)
		}
		response := `{"code":0,"data":{"biz_code":0,"biz_data":{"id":"image-1","status":"SUCCESS","file_name":"photo.webp","file_size":` + strconv.Itoa(len(data)) + `,"is_image":true}}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	result, err := client.UploadFile(context.Background(), &auth.RequestAuth{AccountID: "account"}, UploadFileRequest{Filename: "photo.png", ContentType: "image/png", Data: input.Bytes(), ThinkingEnabled: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "image-1" || !result.IsImage || result.Filename != "photo.webp" {
		t.Fatalf("result mismatch: %#v", result)
	}
}

func TestUploadFailedProcessingDoesNotPoll(t *testing.T) {
	client := &Client{}
	err := client.waitForUploadedFile(context.Background(), &auth.RequestAuth{}, &UploadFileResult{ID: "file-test", Status: "FAILED"})
	if !errors.Is(err, ErrUploadFileRejected) {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fileReadySleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("polling did not honor cancellation: %v", err)
	}
}

func TestMultipartMatchesAPKParameterQuoting(t *testing.T) {
	body, _, err := buildUploadMultipartBody("report.pdf", "application/pdf", []byte("%PDF-test"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("Content-Disposition: form-data; name=file; filename=report.pdf\r\nContent-Type: application/octet-stream\r\n\r\n%PDF-test\r\n")) {
		t.Fatalf("unexpected App part: %s", body)
	}
	if got := appMultipartParameter("report final.pdf"); got != `"report final.pdf"` {
		t.Fatalf("quoted parameter: %q", got)
	}
}
