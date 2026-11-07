package streamwriter

import (
	"context"
	"ds2api/internal/tooldiagnostics"
)

// ToolsWritten is called only after a complete protocol tool item has flushed.
func (s *Writer) ToolsWritten(ctx context.Context, ordinals ...int) bool {
	if s.err != nil {
		return false
	}
	tooldiagnostics.ToolsWritten(ctx, ordinals...)
	return true
}

// Complete is a successful response terminal, independent of later cancellation.
func (s *Writer) Complete(ctx context.Context) bool {
	if s.err != nil {
		return false
	}
	s.completed = true
	tooldiagnostics.ResponseWritten(ctx)
	return true
}

func (s *Writer) Completed() bool { return s.completed && s.err == nil }
