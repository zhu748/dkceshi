package sse

import (
	"strings"
	"testing"
)

// A degenerate upstream streaming an unbounded backtick run must not grow the
// guard's pending buffer without limit (it is re-concatenated on every Feed).
func TestToolRoleMarkerGuardBoundsPendingOnHugeTickRun(t *testing.T) {
	var g ToolRoleMarkerGuard
	chunk := strings.Repeat("`", 4096)
	for i := 0; i < 32; i++ {
		g.Feed(chunk)
	}
	if len(g.pending) > 1024 {
		t.Fatalf("pending grew unbounded: %d bytes", len(g.pending))
	}
	// The run was consumed as a (degenerate) code fence, so text after it is
	// treated as code content and [Tool]: stays literal — the same behavior a
	// normal-length fence produces.
	out := g.Feed("text [Tool]: more")
	if g.tripped {
		t.Fatalf("inside the fence the marker must stay literal, got %q", out)
	}
	// A fresh guard (no fence state) still trips on the marker.
	var fresh ToolRoleMarkerGuard
	fresh.Feed("hello ")
	kept := fresh.Feed("text [Tool]: more")
	if !fresh.tripped || kept != "text " {
		t.Fatalf("fresh guard must cut at [Tool]:, tripped=%v kept=%q", fresh.tripped, kept)
	}
}
