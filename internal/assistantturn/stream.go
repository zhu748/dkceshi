package assistantturn

import (
	"ds2api/internal/sse"
)

type StreamEventType string

const (
	StreamEventTextDelta     StreamEventType = "text_delta"
	StreamEventThinkingDelta StreamEventType = "thinking_delta"
	StreamEventToolCall      StreamEventType = "tool_call"
	StreamEventDone          StreamEventType = "done"
	StreamEventError         StreamEventType = "error"
	StreamEventPing          StreamEventType = "ping"
)

type StreamEvent struct {
	Type     StreamEventType
	Text     string
	Thinking string
	ToolCall any
	Error    *OutputError
	Usage    *Usage
}

type Accumulator struct {
	inner StreamAccumulator
}

type AccumulatorOptions struct {
	Prompt                string
	ThinkingEnabled       bool
	SearchEnabled         bool
	StripReferenceMarkers bool
}

func NewAccumulator(opts AccumulatorOptions) *Accumulator {
	return &Accumulator{
		inner: StreamAccumulator{
			Prompt:                opts.Prompt,
			ThinkingEnabled:       opts.ThinkingEnabled,
			SearchEnabled:         opts.SearchEnabled,
			StripReferenceMarkers: opts.StripReferenceMarkers,
		},
	}
}

func (a *Accumulator) Apply(parsed sse.LineResult) StreamAccumulatorResult {
	if a == nil {
		return StreamAccumulatorResult{}
	}
	return a.inner.Apply(parsed)
}

// ResetContentPolicy starts a new upstream round, see
// StreamAccumulator.ResetContentPolicy.
func (a *Accumulator) ResetContentPolicy() {
	if a == nil {
		return
	}
	a.inner.ResetContentPolicy()
}

func (a *Accumulator) Cut() sse.CutReason {
	if a == nil {
		return sse.CutNone
	}
	return a.inner.Cut()
}

// ResetForRegeneration discards accumulated output before a replacement
// generation starts (an empty-output retry or a switch to another account):
// the previous attempt produced no usable answer, so its partial raw text,
// thinking and tool-detection buffers must not leak into the new one. The
// content policy is reset as well because the new attempt reads a fresh
// upstream response. Mirrors the per-surface resetForRegeneration helpers.
func (a *Accumulator) ResetForRegeneration() {
	if a == nil {
		return
	}
	a.inner.RawText.Reset()
	a.inner.Text.Reset()
	a.inner.RawThinking.Reset()
	a.inner.Thinking.Reset()
	a.inner.ToolDetectionThinking.Reset()
	a.inner.ResetThinkingGuard()
	a.inner.ResetContentPolicy()
}

func (a *Accumulator) SetToolMarker(marker string) {
	if a != nil {
		a.inner.ToolMarker = marker
	}
}

// HasPartialOutput reports whether the accumulated stream contains any raw
// text, thinking or tool-detection content — i.e. the upstream produced
// something before the stream ended. Used to distinguish an interrupted stream
// worth resuming from a cleanly empty one.
func (a *Accumulator) HasPartialOutput() bool {
	if a == nil {
		return false
	}
	return a.inner.HasPartialOutput()
}

func (a *Accumulator) Snapshot() (rawText, text, rawThinking, thinking, detectionThinking string) {
	if a == nil {
		return "", "", "", "", ""
	}
	return a.inner.RawText.String(),
		a.inner.Text.String(),
		a.inner.RawThinking.String(),
		a.inner.Thinking.String(),
		a.inner.ToolDetectionThinking.String()
}
