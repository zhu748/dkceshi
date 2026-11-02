package server

import (
	"github.com/go-chi/chi/v5/middleware"
	"net/http"
	"time"
)

func timeout(d time.Duration) func(http.Handler) http.Handler {
	if d <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return middleware.Timeout(d)
}

// probedMethods lists the HTTP methods we re-probe against the routing tree to
// rebuild the Allow header, because overriding chi's MethodNotAllowed handler
// drops the method list chi would otherwise have attached.
var probedMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}
