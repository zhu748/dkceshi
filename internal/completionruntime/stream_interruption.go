package completionruntime

import streamengine "ds2api/internal/stream"

// StreamInterrupted is shared by protocol adapters; transport EOF without a
// terminal marker is reported as a read error by the SSE pump.
func StreamInterrupted(readErr error, reason streamengine.StopReason) bool {
	return readErr != nil || reason == streamengine.StopReasonIdleTimeout || reason == streamengine.StopReasonNoContentTimeout
}
