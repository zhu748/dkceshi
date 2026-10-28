package transport

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHeaderOrderMatchesAllHARRequestShapes(t *testing.T) {
	raw, err := os.ReadFile("../../../tests/fixtures/android_app_2_7_1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Requests []struct {
			Method, Path  string
			HeaderNames   []string          `json:"header_names"`
			StaticHeaders map[string]string `json:"static_headers"`
		}
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, sample := range fixture.Requests {
		t.Run(sample.Method+sample.Path, func(t *testing.T) {
			headers := sample.StaticHeaders
			for _, name := range sample.HeaderNames {
				if !strings.HasPrefix(name, ":") && headers[name] == "" {
					headers[name] = "test-value"
				}
			}
			headers["content-type"] = "application/json"
			if strings.HasSuffix(sample.Path, "upload_file") {
				headers["content-type"] = "multipart/form-data; boundary=test"
			}
			ordered := BuildOrderedRequestHeaders(sample.Method, sample.Path, headers, 123)
			names := []string{":method", ":authority", ":path", ":scheme"}
			for _, header := range ordered {
				names = append(names, header.Name)
			}
			if !reflect.DeepEqual(names, sample.HeaderNames) {
				t.Fatalf("got %v; want %v", names, sample.HeaderNames)
			}
		})
	}
}

func TestOrderedHeadersFilterAndLength(t *testing.T) {
	headers := map[string]string{"Host": "wrong", ":path": "wrong", "Connection": "keep-alive", "Content-Length": "999", "Accept-Encoding": "gzip", "X-Extra-Z": "z", "x-extra-a": "a", "X-Device-Model": "old", "x-device-model": "new"}
	got := BuildOrderedRequestHeaders(http.MethodPost, "/api/v0/chat_session/create", headers, 0)
	want := []OrderedHeader{{Name: "x-device-model", Value: "new"}, {Name: "content-length", Value: "0"}, {Name: "accept-encoding", Value: "gzip"}, {Name: "x-extra-a", Value: "a"}, {Name: "x-extra-z", Value: "z"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, h := range BuildOrderedRequestHeaders(method, "/api/v0/file/fetch_files", headers, 0) {
			if h.Name == "content-type" || h.Name == "content-length" {
				t.Fatalf("unexpected GET/HEAD body header: %#v", h)
			}
		}
	}
}

func TestOrderedHTTP2DefaultAndOptOut(t *testing.T) {
	t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "")
	t.Setenv("VERCEL", "")
	if !UseFramedH2() {
		t.Fatal("ordered HTTP/2 should be enabled by default")
	}
	t.Setenv("DS2API_DEEPSEEK_USE_FRAMED_H2", "0")
	t.Setenv("VERCEL", "1")
	if UseFramedH2() {
		t.Fatal("explicit opt-out should win")
	}
}
