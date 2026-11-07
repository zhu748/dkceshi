package tooldiagnostics

import (
	"context"
	"time"
)

// ToolsWritten records complete, validated tool items at the output boundary.
// Ordinals are one-based; an empty list means the entire current batch.
func ToolsWritten(ctx context.Context, ordinals ...int) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || !r.active || r.record.CompletedAt != 0 || r.current().Result != "success" {
		return
	}
	changed := false
	for i := range r.current().Calls {
		call := &r.current().Calls[i]
		selected := len(ordinals) == 0
		for _, ordinal := range ordinals {
			selected = selected || call.Ordinal == ordinal
		}
		if selected && call.Parse == "success" && call.Delivery != "written" {
			call.Delivery, changed = "written", true
		}
	}
	if changed {
		r.schedulePersistLocked()
	}
}

// ResponseWritten commits successful terminal-frame evidence before cleanup.
// It does not claim the client executed any tool or acknowledged the response.
func ResponseWritten(ctx context.Context) {
	r := FromContext(ctx)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.matchesAttempt(ctx) || !r.active || r.record.CompletedAt != 0 || r.current().Result != "success" || r.current().ResponseWrittenAt != 0 {
		return
	}
	r.current().ResponseWrittenAt = time.Now().UnixMilli()
	r.persistLocked()
}
