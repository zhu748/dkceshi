package assistantturn

import "ds2api/internal/tokenusage"

// ResolveUsage uses only explicitly named per-message counters. The upstream
// accumulated scalar has no verified input/output scope and remains raw metadata.
// Multiple message counters are retained for inspection, never guessed at as a
// per-request sum: their input contexts may overlap.
func ResolveUsage(base Usage, observed tokenusage.Snapshot, complete bool) Usage {
	estimated := base
	m := &tokenusage.Metadata{Source: "estimated", InputSource: "estimated", OutputSource: "estimated", ReasoningSource: "estimated", TotalSource: "derived", Complete: complete}
	if len(observed.Messages) > 0 || observed.Request != nil {
		snapshot := observed.Clone()
		m.Upstream = &snapshot
	}
	if o, ok := observed.Effective(); ok {
		consistent := o.InputTokens == nil || o.OutputTokens == nil || o.TotalTokens == nil || int64(*o.InputTokens)+int64(*o.OutputTokens) <= tokenusage.MaxCount && int64(*o.TotalTokens) == int64(*o.InputTokens)+int64(*o.OutputTokens)
		if consistent {
			if o.InputTokens != nil {
				base.InputTokens = *o.InputTokens
				m.InputSource = "upstream"
			}
			if o.OutputTokens != nil {
				base.OutputTokens = *o.OutputTokens
				m.OutputSource = "upstream"
			}
			if o.ReasoningTokens != nil && o.OutputTokens == nil {
				if output, ok := tokenusage.Count(int64(base.OutputTokens) - int64(base.ReasoningTokens) + int64(*o.ReasoningTokens)); ok {
					base.OutputTokens = output
					base.ReasoningTokens = *o.ReasoningTokens
					m.OutputSource = "mixed"
					m.ReasoningSource = "upstream"
				}
			}
			if o.ReasoningTokens != nil && *o.ReasoningTokens <= base.OutputTokens {
				base.ReasoningTokens = *o.ReasoningTokens
				m.ReasoningSource = "upstream"
			} else if base.ReasoningTokens > base.OutputTokens {
				base.ReasoningTokens = 0
				m.ReasoningSource = "unknown"
			}
		}
	}
	if _, ok := tokenusage.Count(int64(base.InputTokens) + int64(base.OutputTokens)); !ok {
		// Do not publish an unsafe sum to JavaScript clients.
		base = estimated
		m.InputSource, m.OutputSource, m.ReasoningSource = "estimated", "estimated", "estimated"
	}
	base.TotalTokens = base.InputTokens + base.OutputTokens
	if o, ok := observed.Effective(); ok && o.TotalTokens != nil && *o.TotalTokens == base.TotalTokens && m.InputSource == "upstream" && m.OutputSource == "upstream" {
		m.TotalSource = "upstream"
	}
	if m.InputSource == "upstream" && m.OutputSource == "upstream" {
		m.Source = "upstream"
	} else if m.InputSource == "upstream" || m.OutputSource == "upstream" || m.OutputSource == "mixed" || m.ReasoningSource == "upstream" {
		m.Source = "mixed"
	}
	base.Metadata = m
	return base
}

func usageMetadata(usage Usage) *tokenusage.Metadata {
	if usage.Metadata != nil {
		return usage.Metadata
	}
	return ResolveUsage(usage, tokenusage.Snapshot{}, true).Metadata
}

// Protocol renderers and history reuse the same counters and metadata.
func ClaudeUsage(turn Turn) map[string]any {
	return map[string]any{"input_tokens": turn.Usage.InputTokens, "output_tokens": turn.Usage.OutputTokens, "ds2api": usageMetadata(turn.Usage)}
}

func GeminiUsage(turn Turn) map[string]any {
	u := turn.Usage
	metadata := usageMetadata(u)
	usage := map[string]any{"promptTokenCount": u.InputTokens, "totalTokenCount": u.TotalTokens, "ds2api": metadata}
	if metadata.ReasoningSource != "unknown" {
		usage["candidatesTokenCount"] = u.OutputTokens - u.ReasoningTokens
		usage["thoughtsTokenCount"] = u.ReasoningTokens
	}
	return usage
}
