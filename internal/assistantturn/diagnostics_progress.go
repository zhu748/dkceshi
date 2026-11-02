package assistantturn

import (
	"context"
	"strings"
	"time"

	"ds2api/internal/tooldiagnostics"
	"ds2api/internal/toolstream"
)

type diagnosticProbe struct {
	state toolstream.State
	seen  int
	found bool
}

func (p *diagnosticProbe) feed(raw, marker string) bool {
	if p.found {
		return true
	}
	if len(raw) < p.seen {
		*p = diagnosticProbe{}
	}
	p.state.DeferMalformed, p.state.Marker = true, marker
	events := toolstream.ProcessChunk(&p.state, raw[p.seen:], nil)
	p.seen = len(raw)
	p.found = p.state.HasToolCandidate()
	for _, event := range events {
		p.found = p.found || event.ToolBoundary || len(event.ToolCalls) > 0
	}
	if p.found {
		p.state = toolstream.State{} // stop parsing and release the duplicate buffer
	}
	return p.found
}

// DiagnosticProgress shares literal-aware candidate detection across adapters.
// It never runs repair or final validation. Reset it only on regeneration.
type DiagnosticProgress struct {
	content, reasoning diagnosticProbe
	last               time.Time
	source             string
}

func (p *DiagnosticProgress) Update(ctx context.Context, marker, text, thinking, detection string) {
	if tooldiagnostics.FromContext(ctx) == nil {
		return
	}
	if detection == "" {
		detection = thinking
	}
	source, raw := p.selectSource(marker, text, detection)
	if source == "" || source == p.source && time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.last, p.source = time.Now(), source
	tooldiagnostics.Candidate(ctx, source, tooldiagnostics.Capture(raw))
}

func (p *DiagnosticProgress) selectSource(marker, text, thinking string) (string, string) {
	if p.content.feed(text, marker) {
		return "content", text
	}
	if strings.TrimSpace(text) != "" {
		return "", ""
	}
	if p.reasoning.feed(thinking, marker) {
		return "reasoning", thinking
	}
	return "", ""
}

// Node sends bounded snapshots with original byte counts. Recheck their literal
// context in Go; never trust a transport's tool-name/catalog relevance guess.
func CheckpointToolProgress(ctx context.Context, marker string, text, thinking tooldiagnostics.Text) {
	p := DiagnosticProgress{}
	source, _ := p.selectSource(marker, text.Text, thinking.Text)
	if source == "" {
		return
	}
	raw := text
	if source == "reasoning" {
		raw = thinking
	}
	if len(raw.Text) > tooldiagnostics.MaxTextBytes {
		bounded := tooldiagnostics.Capture(raw.Text)
		bounded.Bytes, bounded.Truncated = max(raw.Bytes, bounded.Bytes), true
		raw = bounded
	}
	tooldiagnostics.Candidate(ctx, source, raw)
}
