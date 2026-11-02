package tooldiagnostics

import (
	"log/slog"
	"time"
)

const recorderFlushInterval = 100 * time.Millisecond

func (r *Recorder) touchLocked() {
	r.record.UpdatedAt = time.Now().UnixMilli()
	r.record.Revision++
	r.record.AttemptCount = len(r.record.Attempts)
}

// First candidates, validation, failures and Finish are synchronous checkpoints.
func (r *Recorder) persistLocked() {
	if !r.active {
		return
	}
	r.touchLocked()
	r.writeLocked()
}
func (r *Recorder) schedulePersistLocked() {
	if !r.active {
		return
	}
	r.touchLocked()
	if r.flushTimer != nil {
		return
	}
	r.flushVersion++
	version := r.flushVersion
	r.flushTimer = time.AfterFunc(recorderFlushInterval, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if version == r.flushVersion {
			r.writeLocked()
		}
	})
}
func (r *Recorder) writeLocked() {
	r.cancelPersistLocked()
	bounded := boundRecord(r.record)
	// Preserve immutable identity fields when the timer compacts evidence.
	r.record.Attempts = bounded.Attempts
	r.record.Tools, r.record.Policy = bounded.Tools, bounded.Policy
	r.record.Error, r.record.EvidenceTruncated = bounded.Error, bounded.EvidenceTruncated
	if err := r.store.Put(r.record); err != nil {
		slog.Warn("[tool_diagnostics] persistence failed", "id", r.record.ID, "error", err)
	}
}

func (r *Recorder) cancelPersistLocked() {
	if r.flushTimer != nil {
		r.flushTimer.Stop()
		r.flushTimer = nil
	}
	r.flushVersion++
}
