package requestbody

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
)

// BodyDigest fingerprints the validated HTTP body, before decoding, caller
// flags, uploads or protocol normalization. It never retains the body itself.
type BodyDigest struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type bodyAuditKey struct{}
type bodyAudit struct {
	mu     sync.Mutex
	digest *BodyDigest
}

func withBodyAudit(r *http.Request) (*http.Request, *bodyAudit) {
	audit := &bodyAudit{}
	return r.WithContext(context.WithValue(r.Context(), bodyAuditKey{}, audit)), audit
}

func (a *bodyAudit) capture(raw []byte) {
	if a == nil {
		return
	}
	sum := sha256.Sum256(raw)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.digest = &BodyDigest{Bytes: len(raw), SHA256: hex.EncodeToString(sum[:])}
}

func RequestBodyDigest(r *http.Request) *BodyDigest {
	if r == nil {
		return nil
	}
	audit, _ := r.Context().Value(bodyAuditKey{}).(*bodyAudit)
	if audit == nil {
		return nil
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if audit.digest == nil {
		return nil
	}
	copy := *audit.digest
	return &copy
}
