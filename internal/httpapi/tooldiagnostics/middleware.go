package tooldiagnostics

import (
	"context"
	"io"
	"net/http"
	"strings"

	"ds2api/internal/tooldiagnostics"
)

func Middleware(store *tooldiagnostics.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !completionPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(tooldiagnostics.WithRepairSettings(r.Context(), store.RepairSettings()))
			if internalRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			recorder := tooldiagnostics.NewRecorder(store, r.URL.Path, false)
			if recorder == nil {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(tooldiagnostics.WithRecorder(r.Context(), recorder))
			capture := &responseWriter{ResponseWriter: w, status: 200, ctx: r.Context()}
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
	for _, key := range []string{"__stream_prepare", "__stream_validate", "__stream_release", "__stream_pow", "__stream_switch", "__stream_progress"} {
		if r.URL.Query().Get(key) == "1" {
			return true
		}
	}
	return false
}

type responseWriter struct {
	ctx context.Context
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

// JSON encoders notify the boundary after the entire payload has been written.
func (w *responseWriter) JSONResponseWritten(err error) {
	if err != nil {
		w.err = err
		return
	}
	if w.status < 400 && w.err == nil {
		tooldiagnostics.ToolsWritten(w.ctx)
		tooldiagnostics.ResponseWritten(w.ctx)
	}
}
