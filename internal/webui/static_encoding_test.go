package webui

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func compressedFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte(strings.Repeat("console.log('panel');", 1000))
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(root, "assets", "index-test.js")
	if err := os.WriteFile(full, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full+".gz", buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-time.Hour)
	for _, p := range []string{full, full + ".gz"} {
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	return root, content, buf.Bytes()
}

func TestAdminAssetsNegotiatePrecompressedRepresentation(t *testing.T) {
	root, content, compressed := compressedFixture(t)
	h := &Handler{StaticDir: root}
	for _, tc := range []struct {
		encoding string
		gzip     bool
	}{
		{"", false}, {"br, gzip, deflate", true}, {"GZIP; q=0.5", true},
		{"gzip;q=0, *;q=1", false}, {"br,*;q=0.5", true}, {"gzip;q=invalid", false},
		{"gzip;q=-1", false}, {"gzip;q=2", false}, {"gzip;q=NaN", false},
	} {
		t.Run(tc.encoding, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin/assets/index-test.js", nil)
			req.Header.Set("Accept-Encoding", tc.encoding)
			rec := httptest.NewRecorder()
			h.serveFromDisk(rec, req, root)
			if rec.Code != 200 || rec.Header().Get("Vary") != "Accept-Encoding" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
				t.Fatalf("bad headers: %d %#v", rec.Code, rec.Header())
			}
			body := rec.Body.Bytes()
			if tc.gzip {
				if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != strconv.Itoa(len(compressed)) {
					t.Fatal("gzip representation not selected")
				}
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				body, err = io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					t.Fatal(err)
				}
			} else if rec.Header().Get("Content-Encoding") != "" {
				t.Fatal("unexpected encoding")
			}
			if !bytes.Equal(body, content) {
				t.Fatal("response content changed")
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
				t.Fatal("lost asset cache policy")
			}
		})
	}
}

func TestAdminAssetRangesAndStaleSidecarsUseOriginal(t *testing.T) {
	root, content, _ := compressedFixture(t)
	h := &Handler{StaticDir: root}
	req := httptest.NewRequest("GET", "/admin/assets/index-test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-9")
	rec := httptest.NewRecorder()
	h.serveFromDisk(rec, req, root)
	if rec.Code != 206 || !bytes.Equal(rec.Body.Bytes(), content[:10]) || rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("range was applied to compressed offsets")
	}
	original := filepath.Join(root, "assets", "index-test.js")
	stamp := time.Now()
	if err := os.Chtimes(original, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	req.Header.Del("Range")
	rec = httptest.NewRecorder()
	h.serveFromDisk(rec, req, root)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "" || !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatal("stale compressed data was served")
	}
	if err := os.Remove(original + ".gz"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.serveFromDisk(rec, req, root)
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatal("source-only builds stopped working")
	}
}

func TestCompressedHeadAndConditionalRequests(t *testing.T) {
	root, _, compressed := compressedFixture(t)
	h := &Handler{StaticDir: root}
	req := httptest.NewRequest("HEAD", "/admin/assets/index-test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.serveFromDisk(rec, req, root)
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != strconv.Itoa(len(compressed)) {
		t.Fatalf("invalid HEAD response: %d %#v", rec.Code, rec.Header())
	}
	modified := rec.Header().Get("Last-Modified")
	if modified == "" {
		t.Fatal("conditional caching unavailable")
	}
	req = httptest.NewRequest("GET", "/admin/assets/index-test.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-Modified-Since", modified)
	rec = httptest.NewRecorder()
	h.serveFromDisk(rec, req, root)
	if rec.Code != 304 || rec.Body.Len() != 0 || rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("invalid conditional response: %d %#v", rec.Code, rec.Header())
	}
}
