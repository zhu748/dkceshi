package sse

// CutReason identifies why a content policy stopped forwarding raw upstream
// text. It is the single vocabulary shared by every protocol surface and every
// runtime: a new cut rule is declared here once and reported by all of them
// without per-surface changes.
type CutReason string

const (
	// CutNone means the policy is still forwarding.
	CutNone CutReason = ""
	// CutRepeatLoop means the upstream text degenerated into a run of protocol
	// closing tags (see RepeatLoopGuard).
	CutRepeatLoop CutReason = "repeat_loop"
	// CutToolRoleMarker means the upstream hallucinated the `[Tool]:` role
	// marker; the marker and everything after it is fabricated dialogue (see
	// ToolRoleMarkerGuard).
	CutToolRoleMarker CutReason = "tool_role_marker"
	// CutTranscriptBoundary ends the model's turn at the serialized terminator,
	// before it can invent subsequent assistant, user or tool messages.
	CutTranscriptBoundary CutReason = "transcript_boundary"
	// CutPromptEcho rejects a reply that restarts the request's own generated
	// system prelude or serializer introduction instead of answering.
	CutPromptEcho CutReason = "prompt_echo"
)

// LogMessage returns the sentence describing this cut in the "cutting stream"
// warning, so every surface logs the same words for the same rule.
func (r CutReason) LogMessage() string {
	switch r {
	case CutRepeatLoop:
		return "upstream repetition loop detected; cutting stream"
	case CutToolRoleMarker:
		return "hallucinated [Tool]: role marker detected; cutting stream"
	case CutTranscriptBoundary:
		return "serialized message boundary detected; cutting stream"
	case CutPromptEcho:
		return "generated prompt echoed in answer; rejecting stream"
	}
	return ""
}

// ContentPolicy is the ordered set of cut rules applied to the raw text channel
// of one upstream stream, plus the bookkeeping that makes a cut sticky.
//
// Text is fed in arbitrary SSE fragments. Feed returns the prefix that is safe
// to forward downstream; once any rule fires it returns "" forever, and Cut
// reports which rule fired so the caller can end the upstream read and run its
// normal finish path.
//
// Rules are evaluated in declaration order and the first one that fires on a
// fragment wins, so a fragment that both repeats closing tags and introduces a
// role marker reports the repetition loop.
//
// Adding a cut rule is a change to this file alone: declare the guard, feed it
// in Feed, and give it a CutReason. Every surface picks it up through Cut with
// no edit of its own.
type ContentPolicy struct {
	Prompt string

	echo     PromptEchoGuard
	echoInit bool
	repeat   RepeatLoopGuard
	toolRole ToolRoleMarkerGuard
	cut      CutReason
}

// Feed consumes the next raw text fragment and returns the prefix that is safe
// to forward downstream. A trailing fragment that could still grow into a
// matched pattern is withheld until a later Feed.
func (p *ContentPolicy) Feed(text string) string {
	if p == nil || p.cut != CutNone {
		return ""
	}
	if !p.echoInit {
		p.echo = NewPromptEchoGuard(p.Prompt)
		p.echoInit = true
	}
	text = p.echo.Feed(text)
	if p.echo.tripped {
		p.cut = CutPromptEcho
		return ""
	}
	kept := p.toolRole.Feed(text)
	roleCut := p.toolRole.Tripped()
	if kept != "" {
		p.repeat.Feed(kept)
	}
	switch {
	case p.repeat.Tripped():
		p.cut = CutRepeatLoop
	case roleCut:
		p.cut = p.toolRole.Reason()
	}
	return kept
}

// Flush finishes pending literal text once, without turning incomplete
// fingerprints or delimiters into a false positive.
func (p *ContentPolicy) Flush() string {
	if p == nil || p.cut != CutNone {
		return ""
	}
	tail := p.echo.Flush()
	if tail == "" && p.toolRole.code == 0 && !p.toolRole.inCDATA && !p.toolRole.quote {
		return ""
	}
	return p.toolRole.feed(tail, true)
}

// Cut reports which rule ended forwarding, or CutNone while the policy is still
// forwarding.
func (p *ContentPolicy) Cut() CutReason {
	if p == nil {
		return CutNone
	}
	return p.cut
}
