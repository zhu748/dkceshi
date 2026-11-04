package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestLatestGIFHARHeaderShapesOnActualHTTP2Wire(t *testing.T) {
	for _, filename := range []string{"android_app_2_7_1_gif_reuse.json", "android_app_2_7_1_2026_11_04.json", "android_app_2_7_1_login_2026_11_04.json"} {
		t.Run(filename, func(t *testing.T) { testCurrentHARWire(t, filename) })
	}
}

func testCurrentHARWire(t *testing.T, filename string) {
	t.Helper()
	var fixture struct {
		InstallationHeaderMode string `json:"installation_header_mode"`
		Shapes                 []struct {
			Method   string            `json:"method"`
			Path     string            `json:"path"`
			Names    []string          `json:"header_names"`
			Static   map[string]string `json:"static_headers"`
			BodyKind string            `json:"body_kind"`
		} `json:"request_shapes"`
	}
	raw, err := os.ReadFile("../../../tests/fixtures/" + filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	// Earlier samples omit this optional header; the startup/login sample
	// preserves it once the App has an installation identity available.
	mode := "0"
	if fixture.InstallationHeaderMode == "preserve" {
		mode = ""
	}
	t.Setenv("DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER", mode)
	for _, shape := range fixture.Shapes {
		t.Run(shape.Path, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := listener.Close(); err != nil {
					t.Log(err)
				}
			})
			observed := make(chan observedRequest, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					observed <- observedRequest{err: err}
					return
				}
				observed <- receiveOrderedRequest(conn)
			}()
			var body []byte
			switch shape.BodyKind {
			case "json":
				body = []byte(`{"prompt":"测试"}`)
			case "encrypted":
				// Exercise opaque body transport, not App encryption generation.
				body = []byte{0, 1, 2, 0xff, 0x80, 3}
			case "multipart":
				body = []byte("--fixture\r\nContent-Disposition: form-data; name=file; filename=animation.gif\r\nContent-Type: application/octet-stream\r\n\r\nGIF89a\r\n--fixture--\r\n")
			}
			req, err := http.NewRequest(shape.Method, "http://app.test"+shape.Path, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range shape.Names {
				if name[0] == ':' {
					continue
				}
				value := shape.Static[name]
				if value == "" {
					value = "synthetic-test-value"
				}
				if name == "content-type" {
					if shape.Static[name] == "" {
						value = "application/json"
					}
					if shape.BodyKind == "multipart" {
						value = "multipart/form-data; boundary=fixture"
					}
				}
				req.Header.Set(name, value)
			}
			if mode == "0" {
				req.Header.Set("x-device-id", "local-installation")
			}
			req.Header.Set("Accept-Language", "*")
			req.Header.Set("sec-fetch-mode", "cors")
			client := &H2FramedClient{Timeout: 5 * time.Second, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
			}}
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, response.Body); err != nil {
				t.Error(err)
			}
			if err := response.Body.Close(); err != nil {
				t.Error(err)
			}
			got := <-observed
			if got.err != nil {
				t.Fatal(got.err)
			}
			var names []string
			values := map[string]string{}
			for _, h := range got.fields {
				names = append(names, h.Name)
				values[h.Name] = h.Value
			}
			if !reflect.DeepEqual(names, shape.Names) || !bytes.Equal(got.body, body) {
				t.Fatalf("wire mismatch: %v want %v", names, shape.Names)
			}
			for name, want := range shape.Static {
				if values[name] != want {
					t.Errorf("%s=%q want %q", name, values[name], want)
				}
			}
			if values["content-length"] != "" && values["content-length"] != strconv.Itoa(len(body)) {
				t.Fatal("JSON Content-Length does not count UTF-8 bytes")
			}
		})
	}
}

func TestInstallationHeaderOverrideIsExplicit(t *testing.T) {
	for _, value := range []string{"", "1", "true", "yes", "on", " TRUE ", "0", "false", "no", "off", "invalid"} {
		t.Setenv("DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER", value)
		got := BuildOrderedRequestHeaders("GET", "/api/v0/file/fetch_files", map[string]string{"x-device-id": "local-id"}, 0)
		found := false
		for _, header := range got {
			found = found || header.Name == "x-device-id"
		}
		want := value != "0" && value != "false" && value != "no" && value != "off"
		if found != want {
			t.Fatalf("override %q: %v", value, got)
		}
	}
}
