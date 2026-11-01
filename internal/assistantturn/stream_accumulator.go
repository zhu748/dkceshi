package assistantturn

import (
	"strings"

	"ds2api/internal/sse"
	"ds2api/internal/textclean"
	"ds2api/internal/toolstream"
)

type StreamAccumulator struct {
	Prompt                string
	ThinkingEnabled       bool
	SearchEnabled         bool
	StripReferenceMarkers bool
	ToolMarker            string

	RawThinking           strings.Builder
	Thinking              strings.Builder
	ToolDetectionThinking strings.Builder
	RawText               strings.Builder
	Text                  strings.Builder

	// policy is the set of cut rules applied to the raw text channel — the
	// upstream repetition loop (a long run of protocol closing tags such as
	// `</|EPSE>` or `</|EPSE|invoke>`) and the hallucinated `[Tool]:` role
	// marker — so the surface can stop reading and keep whatever was generated
	// instead of waiting out the idle watchdog. The rules live in sse so every
	// runtime shares one definition.
	policy        sse.ContentPolicy
	thinkingGuard toolstream.VisibleGuard
}

type StreamPartDelta struct {
	Type         string
	RawText      string
	VisibleText  string
	CitationOnly bool
}

type StreamAccumulatorResult struct {
	ContentSeen bool
	Parts       []StreamPartDelta
	// Cut reports that a content rule ended the raw text channel. The
	// already-accumulated content stays valid and everything from the cut on
	// was dropped; the caller should stop consuming upstream and run its normal
	// finish path. CutNone means the stream is still healthy.
	Cut sse.CutReason
}

func (a *StreamAccumulator) Apply(parsed sse.LineResult) StreamAccumulatorResult {
	out := StreamAccumulatorResult{}
	if parsed.OutputLimit {
		a.policy.RejectOutputLimit()
	}
	for _, p := range parsed.ToolDetectionThinkingParts {
		trimmed := sse.ContentPartDelta(&a.ToolDetectionThinking, p)
		if trimmed != "" {
			if !a.allowOutput(len(trimmed)) {
				break
			}
			a.ToolDetectionThinking.WriteString(trimmed)
			// Tool-detection thinking is real upstream activity even when the
			// caller disabled visible reasoning; without this, a long silent
			// thinking phase would trip the no-content keepalive window and
			// truncate the stream.
			out.ContentSeen = true
		}
	}
	for _, p := range parsed.Parts {
		if p.Text != "" {
			out.ContentSeen = true
		}
		if p.Type == "thinking" {
			delta := a.applyThinkingPart(sse.ContentPartDelta(&a.RawThinking, p))
			if delta.RawText != "" {
				out.ContentSeen = true
			}
			if delta.RawText != "" || delta.VisibleText != "" {
				out.Parts = append(out.Parts, delta)
			}
			continue
		}
		delta := a.applyTextPart(sse.ContentPartDelta(&a.RawText, p))
		if delta.RawText != "" {
			out.ContentSeen = true
		}
		if delta.RawText != "" || delta.VisibleText != "" || delta.CitationOnly {
			out.Parts = append(out.Parts, delta)
		}
	}
	if parsed.Flush {
		if text := a.thinkingGuard.Flush(); text != "" {
			delta := a.appendVisibleThinking(text)
			out.Parts = append(out.Parts, delta)
		}
		delta := a.appendTextPart(a.policy.Flush())
		if delta.RawText != "" {
			out.Parts = append(out.Parts, delta)
		}
	}
	out.Cut = a.policy.Cut()
	return out
}

// Cut reports which content rule ended the raw text channel, or sse.CutNone
// while the stream is still healthy.
func (a *StreamAccumulator) Cut() sse.CutReason {
	if a == nil {
		return sse.CutNone
	}
	return a.policy.Cut()
}

// ResetContentPolicy starts a replacement generation. Resuming the same
// message must retain pending delimiters and CDATA/Markdown state.
func (a *StreamAccumulator) ResetContentPolicy() {
	if a == nil {
		return
	}
	a.policy = sse.ContentPolicy{Prompt: a.Prompt}
}

// HasPartialOutput reports whether the accumulated stream contains any raw
// text, thinking or tool-detection content — i.e. the upstream produced
// something before the stream ended. Used to distinguish an interrupted stream
// worth resuming from a cleanly empty one.
func (a *StreamAccumulator) HasPartialOutput() bool {
	if a == nil {
		return false
	}
	return a.RawText.Len() > 0 || a.RawThinking.Len() > 0 || a.ToolDetectionThinking.Len() > 0
}

func (a *StreamAccumulator) applyThinkingPart(text string) StreamPartDelta {
	rawTrimmed := text
	if !a.allowOutput(len(rawTrimmed)) {
		return StreamPartDelta{Type: "thinking"}
	}
	if rawTrimmed != "" {
		a.RawThinking.WriteString(rawTrimmed)
	}
	delta := StreamPartDelta{Type: "thinking", RawText: rawTrimmed}
	if !a.ThinkingEnabled || rawTrimmed == "" {
		return delta
	}
	a.thinkingGuard.Prompt = a.Prompt
	a.thinkingGuard.Marker = a.ToolMarker
	a.thinkingGuard.StripReferenceMarkers = a.StripReferenceMarkers
	return a.appendVisibleThinkingDelta(delta, a.thinkingGuard.Write(rawTrimmed))
}

// ResetThinkingGuard discards a failed generation's held protocol fragments.
// Continuations keep the guard so split tags cannot escape across attempts.
func (a *StreamAccumulator) ResetThinkingGuard() {
	a.thinkingGuard = toolstream.VisibleGuard{Marker: a.ToolMarker}
}

func (a *StreamAccumulator) appendVisibleThinking(text string) StreamPartDelta {
	return a.appendVisibleThinkingDelta(StreamPartDelta{Type: "thinking"}, text)
}

func (a *StreamAccumulator) appendVisibleThinkingDelta(delta StreamPartDelta, text string) StreamPartDelta {
	cleanedText := textclean.CleanVisibleDelta(text, a.StripReferenceMarkers)
	if cleanedText == "" {
		return delta
	}
	trimmed := cleanedText
	if trimmed == "" {
		return delta
	}
	a.Thinking.WriteString(trimmed)
	delta.VisibleText = trimmed
	return delta
}

func (a *StreamAccumulator) applyTextPart(text string) StreamPartDelta {
	rawTrimmed := text
	if rawTrimmed == "" {
		return StreamPartDelta{Type: "text"}
	}
	// A content rule may end the raw text channel here — a hallucinated
	// `[Tool]:` role marker and everything after it is fabricated dialogue, and
	// a repetition loop means the rest of the stream is degenerate. Either way
	// the text produced before the cut is kept and the rest is dropped.
	a.policy.Prompt = a.Prompt
	if !a.allowOutput(len(rawTrimmed)) {
		return StreamPartDelta{Type: "text"}
	}
	rawTrimmed = a.policy.Feed(rawTrimmed)
	return a.appendTextPart(rawTrimmed)
}

func (a *StreamAccumulator) appendTextPart(rawTrimmed string) StreamPartDelta {
	if rawTrimmed == "" {
		return StreamPartDelta{Type: "text"}
	}
	a.RawText.WriteString(rawTrimmed)
	delta := StreamPartDelta{Type: "text", RawText: rawTrimmed}
	// Keep the assembled body lossless here. The body sieve owns streaming
	// cleanup; finalization cleans this complete snapshot with full context.
	cleanedText := textclean.CleanVisibleDelta(rawTrimmed, a.StripReferenceMarkers)
	trimmed := cleanedText
	if trimmed == "" {
		return delta
	}
	a.Text.WriteString(trimmed)
	delta.VisibleText = trimmed
	return delta
}

func (a *StreamAccumulator) allowOutput(n int) bool {
	return a.policy.AllowOutputSize(a.RawText.Len() + a.RawThinking.Len() + a.ToolDetectionThinking.Len() + n)
}
