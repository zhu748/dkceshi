package tooldiagnostics

import (
	"io"
	"net/http"
	"strings"

	"ds2api/internal/tooldiagnostics"
)

func Middleware(store *tooldiagnostics.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !completionPath(r.URL.Path) || internalRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(tooldiagnostics.WithRepairSettings(r.Context(), store.RepairSettings()))
			recorder := tooldiagnostics.NewRecorder(store, r.URL.Path, false)
			if recorder == nil {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(tooldiagnostics.WithRecorder(r.Context(), recorder))
			capture := &responseWriter{ResponseWriter: w, status: 200}
			var wrapped http.ResponseWriter = capture
			if _, ok := w.(http.Flusher); ok {
				wrapped = &flushWriter{capture}
			}
			defer func() { recorder.Finish(capture.status, capture.err, r.Context().Err() != nil) }()
			next.ServeHTTP(wrapped, r)
		})
	}
}

func internalRequest(r *http.Request) bool {
	for key := range r.URL.Query() {
		if strings.HasPrefix(key, "__stream_") {
			return true
		}
	}
	return false
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	err         error
}

func (w *responseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(200)
	}
	n, err := w.ResponseWriter.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

type flushWriter struct{ *responseWriter }

func (w *flushWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil {
		w.err = err
	}
	return err
}
func (w *flushWriter) Flush() { _ = w.FlushError() }

func completionPath(path string) bool {
	switch path {
	case "/v1/chat/completions", "/chat/completions", "/v1/responses", "/responses", "/anthropic/v1/messages", "/v1/messages", "/messages":
		return true
	}
	return (strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/")) && (strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent"))
}
