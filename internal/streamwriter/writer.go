// Package streamwriter owns transport failure handling for every SSE adapter.
package streamwriter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/streambudget"
)

// Writer is request-local. A failed output is terminal and cannot be retried
// on the same socket; callers stop consuming upstream after Err becomes set.
type Writer struct{ err error }

func (s *Writer) Err() error { return s.err }

func (s *Writer) fail(err error) bool {
	if s.err == nil {
		s.err = err
		config.Logger.Warn("[stream_output] delivery failed", "error", err)
	}
	return false
}

func (s *Writer) JSON(w http.ResponseWriter, rc *http.ResponseController, flush bool, event string, payload any) bool {
	if s.err != nil {
		return false
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return s.fail(err)
	}
	frame := make([]byte, 0, len(data)+len(event)+18)
	if event != "" {
		frame = append(frame, "event: "...)
		frame = append(frame, event...)
		frame = append(frame, '\n')
	}
	frame = append(frame, "data: "...)
	frame = append(frame, data...)
	frame = append(frame, '\n', '\n')
	return s.Write(w, rc, flush, frame)
}

func (s *Writer) Write(w http.ResponseWriter, rc *http.ResponseController, flush bool, frame []byte) bool {
	if s.err != nil {
		return false
	}
	if rc == nil {
		rc = http.NewResponseController(w)
	}
	deadline := time.Now().Add(time.Duration(streambudget.Value("connect_timeout_ms")) * time.Millisecond)
	if err := rc.SetWriteDeadline(deadline); err == nil {
		defer func() {
			if err := rc.SetWriteDeadline(time.Time{}); err != nil {
				config.Logger.Warn("[stream_output] clear write deadline failed", "error", err)
			}
		}()
	} else if !errors.Is(err, http.ErrNotSupported) {
		return s.fail(err)
	}
	n, err := w.Write(frame)
	if err != nil {
		return s.fail(err)
	}
	if n != len(frame) {
		return s.fail(io.ErrShortWrite)
	}
	if flush {
		if err := rc.Flush(); err != nil {
			return s.fail(err)
		}
	}
	return true
}
