package requestbody

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyDigestCapturesOriginalValidatedBytesLazily(t *testing.T) {
	const raw = "  {\"id\":9007199254740993,\"text\":\"private\"}\n"
	h := ValidateJSONUTF8(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if RequestBodyDigest(r) != nil {
			t.Fatal("body read before downstream requested it")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != raw {
			t.Fatalf("body changed: %q %v", body, err)
		}
		digest := RequestBodyDigest(r)
		if digest == nil || digest.Bytes != len(raw) || digest.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(raw))) {
			t.Fatalf("incorrect raw digest: %+v", digest)
		}
		digest.SHA256 = "mutated"
		if RequestBodyDigest(r).SHA256 == "mutated" {
			t.Fatal("digest aliases mutable request state")
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw)))
}

func TestInvalidBodyHasNoValidatedDigest(t *testing.T) {
	h := ValidateJSONUTF8(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err == nil || RequestBodyDigest(r) != nil {
			t.Fatal("invalid JSON acquired a validated digest")
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"incomplete":`)))
}
