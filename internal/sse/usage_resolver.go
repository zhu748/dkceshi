package sse

import (
	"context"
	"ds2api/internal/tokenusage"
	"io"
	"log/slog"
)

func resolveBodyUsage(ctx context.Context, body io.Reader, id int) *tokenusage.Observation {
	resolver, ok := body.(tokenusage.Resolver)
	if !ok || ctx.Err() != nil || id <= 0 {
		return nil
	}
	usage, err := resolver.ResolveUpstreamUsage(ctx, id)
	if err != nil {
		slog.Debug("[usage] upstream lookup unavailable; retain fallback", "error", err)
		return nil
	}
	return &usage
}

func (line LineResult) ApplyUsage(snapshot *tokenusage.Snapshot) {
	if line.ResolvedUsage {
		snapshot.SetRequest(line.UpstreamUsage)
	} else {
		snapshot.Observe(line.UpstreamUsage)
	}
}

type bodyUsageForwarder struct {
	body      io.Reader
	messageID int
	terminal  bool
}

func (f *bodyUsageForwarder) forward(ctx context.Context, line *LineResult, out chan<- LineResult) error {
	// Metadata-only frames must survive text batching and terminal reconstruction.
	if !line.UpstreamUsage.Empty() || line.UpstreamUsage.MessageID > 0 {
		select {
		case out <- LineResult{Parsed: true, UpstreamUsage: line.UpstreamUsage, NextType: line.NextType}:
		case <-ctx.Done():
			return ctx.Err()
		}
		line.UpstreamUsage = tokenusage.Observation{}
	}
	if line.ResponseMessageID > 0 {
		f.messageID = line.ResponseMessageID
	}
	if !line.Stop || f.terminal {
		return nil
	}
	f.terminal = true
	if line.ErrorMessage != "" || line.ContentFilter {
		return nil
	}
	observed := resolveBodyUsage(ctx, f.body, f.messageID)
	if observed == nil {
		return nil
	}
	select {
	case out <- LineResult{Parsed: true, ResolvedUsage: true, UpstreamUsage: *observed, NextType: line.NextType}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
