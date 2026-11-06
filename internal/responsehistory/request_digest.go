package responsehistory

import (
	"net/http"

	"ds2api/internal/httpapi/requestbody"
	"ds2api/internal/promptcompat"
)

// Retain the original fingerprint across internal prepare/release requests and
// account replay. A release body must never replace the client's input digest.
func WithRequestBodyDigest(r *http.Request, standard promptcompat.StandardRequest) promptcompat.StandardRequest {
	if standard.RawRequest == nil {
		if raw := requestbody.RequestBodyDigest(r); raw != nil {
			standard.RawRequest = &promptcompat.ContextDigest{Bytes: raw.Bytes, SHA256: raw.SHA256}
		}
	}
	return standard
}
