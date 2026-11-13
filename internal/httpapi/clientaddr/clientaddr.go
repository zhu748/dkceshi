// Package clientaddr preserves the connection peer before proxy headers rewrite
// RemoteAddr, so authentication throttles cannot be bypassed with forged headers.
package clientaddr

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
)

type peerKey struct{}

func Capture(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), peerKey{}, r.RemoteAddr)))
	})
}

func IP(r *http.Request) string {
	remote := r.RemoteAddr
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("DS2API_TRUST_PROXY_HEADERS")), "true") && os.Getenv("DS2API_TRUST_PROXY_HEADERS") != "1" {
		if peer, ok := r.Context().Value(peerKey{}).(string); ok {
			remote = peer
		}
	}
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}
