package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type observedRequest struct {
	fields []hpack.HeaderField
	body   []byte
	err    error
}

func receiveOrderedRequest(conn net.Conn) (observed observedRequest) {
	defer closeTransportConn(conn)
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		observed.err = err
		return
	}
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		observed.err = err
		return
	}
	if string(preface) != http2.ClientPreface {
		observed.err = errors.New("invalid HTTP/2 preface")
		return
	}
	framer := http2.NewFramer(conn, conn)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	if err := framer.WriteSettings(http2.Setting{ID: http2.SettingInitialWindowSize, Val: 16384}); err != nil {
		observed.err = err
		return
	}
	var requestEnded, settingsAck bool
	var requestStream uint32
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			observed.err = err
			return
		}
		var ended bool
		var stream uint32
		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if f.IsAck() {
				settingsAck = true
			} else {
				if err := framer.WriteSettingsAck(); err != nil {
					observed.err = err
					return
				}
			}
		case *http2.MetaHeadersFrame:
			observed.fields = append(observed.fields, f.Fields...)
			ended, stream = f.StreamEnded(), f.StreamID
		case *http2.DataFrame:
			observed.body = append(observed.body, f.Data()...)
			ended, stream = f.StreamEnded(), f.StreamID
			if len(f.Data()) > 0 {
				if err := framer.WriteWindowUpdate(0, uint32(len(f.Data()))); err != nil {
					observed.err = err
					return
				}
				if err := framer.WriteWindowUpdate(f.StreamID, uint32(len(f.Data()))); err != nil {
					observed.err = err
					return
				}
			}
		}
		if ended {
			requestEnded, requestStream = true, stream
		}
		// Read the client's initial SETTINGS acknowledgement before closing.
		// Closing a TCP socket with unread control frames can reset the peer
		// on Windows and discard the tiny response sent by this test server.
		if requestEnded && settingsAck {
			var block bytes.Buffer
			encoder := hpack.NewEncoder(&block)
			if err := encoder.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}); err != nil {
				observed.err = err
				return
			}
			if err := framer.WriteHeaders(http2.HeadersFrameParam{StreamID: requestStream, BlockFragment: block.Bytes(), EndHeaders: true}); err != nil {
				observed.err = err
				return
			}
			observed.err = framer.WriteData(requestStream, true, []byte("ok"))
			return
		}
	}
}

func TestOrderedH2WireAndLargeUploadFlowControl(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, mime string
		body                     []byte
		wantLength               string
	}{
		{name: "fetch", method: "GET", path: "/api/v0/file/fetch_files?file_ids=file-test"},
		{name: "empty-create", method: "POST", path: "/api/v0/chat_session/create", mime: "application/json", wantLength: "0"},
		{name: "completion", method: "POST", path: "/api/v0/chat/completion", mime: "application/json", body: []byte(`{"prompt":"hello"}`), wantLength: "18"},
		{name: "large-multipart", method: "POST", path: "/api/v0/file/upload_file", mime: "multipart/form-data; boundary=test", body: bytes.Repeat([]byte("x"), 1<<20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			client := &H2FramedClient{Timeout: 5 * time.Second, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
			}}
			var reader io.Reader
			if tc.body != nil {
				reader = bytes.NewReader(tc.body)
			}
			req, err := http.NewRequest(tc.method, "http://app.test"+tc.path, reader)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("User-Agent", "DeepSeek/2.7.1 Android/36")
			req.Header.Set("Accept-Encoding", "gzip")
			if tc.mime != "" {
				req.Header.Set("Content-Type", tc.mime)
			}
			if strings.HasPrefix(tc.mime, "multipart/") {
				// An unknown-length body is how the captured multipart request is sent.
				req.ContentLength = -1
				req.Header.Set("x-ds-pow-response", "pow")
				req.Header.Set("x-thinking-enabled", "1")
				req.Header.Set("x-file-size", strconv.Itoa(len(tc.body)))
				req.Header.Set("x-model-type", "default")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if string(body) != "ok" {
				t.Fatalf("response = %q", body)
			}
			seen := <-observed
			if seen.err != nil {
				t.Fatal(seen.err)
			}
			if !bytes.Equal(seen.body, tc.body) {
				t.Fatalf("body mismatch: %d != %d", len(seen.body), len(tc.body))
			}
			var names []string
			values := map[string]string{}
			for _, field := range seen.fields {
				names = append(names, field.Name)
				values[field.Name] = field.Value
			}
			wantNames := []string{":method", ":authority", ":path", ":scheme"}
			if strings.HasPrefix(tc.mime, "multipart/") {
				wantNames = append(wantNames, "x-ds-pow-response", "x-thinking-enabled", "x-file-size", "x-model-type")
			}
			wantNames = append(wantNames, "user-agent")
			if tc.mime != "" {
				wantNames = append(wantNames, "content-type")
			}
			if tc.wantLength != "" {
				wantNames = append(wantNames, "content-length")
			}
			wantNames = append(wantNames, "accept-encoding")
			if !reflect.DeepEqual(names, wantNames) {
				t.Fatalf("wire headers %v != %v", names, wantNames)
			}
			if values["content-length"] != tc.wantLength || values[":path"] != tc.path {
				t.Fatalf("wrong length/path: %v", values)
			}
		})
	}
}

func TestOrderedH2CancellationBeforeResponseHeaders(t *testing.T) {
	local, remote := net.Pipe()
	defer closeTransportConn(remote)
	ctx, cancel := context.WithCancel(context.Background())
	client := &H2FramedClient{DialContext: func(context.Context, string, string) (net.Conn, error) { return local, nil }}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://app.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Do(req); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("request did not stop on cancellation")
	}
}
