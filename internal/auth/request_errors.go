package auth

import "net/http"

// RequestErrorStatus keeps account availability errors consistent across API
// adapters. A valid caller exhausting the pool has not lost its credentials.
func RequestErrorStatus(err error) int {
	if IsNoIdleAccountError(err) {
		return http.StatusTooManyRequests
	}
	return http.StatusUnauthorized
}
