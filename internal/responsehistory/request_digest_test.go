package responsehistory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/chathistory"
	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/promptcompat"
)

func TestOriginalBodyDigestSurvivesInternalRelease(t *testing.T) {
	const raw = " {\"model\":\"test\",\"input\":\"PRIVATE_TEXT\"} "
	var standard promptcompat.StandardRequest
	requestbody.ValidateJSONUTF8(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatal(err)
		}
		standard = WithRequestBodyDigest(r, standard)
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw)))
	history := chathistory.New(filepath.Join(t.TempDir(), "history.json"))
	requestbody.ValidateJSONUTF8(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Fatal(err)
		}
		session := start(StartParams{Store: history, Request: r, Auth: &auth.RequestAuth{CallerID: "caller"}, Standard: standard})
		entry, err := history.Get(session.entryID)
		if err != nil {
			t.Fatal(err)
		}
		var manifest promptcompat.ContextManifest
		if err := json.Unmarshal(entry.ContextManifest, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.RawRequest == nil || manifest.RawRequest.Bytes != len(raw) || manifest.RawRequest.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) {
			t.Fatalf("release replaced the original input digest: %+v", manifest.RawRequest)
		}
		if strings.Contains(string(entry.ContextManifest), "PRIVATE_TEXT") {
			t.Fatal("manifest leaked input contents")
		}
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/chat/completions?__stream_release=1", strings.NewReader(`{"lease_id":"different-body"}`)))
}
