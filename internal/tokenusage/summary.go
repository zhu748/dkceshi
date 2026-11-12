package tokenusage

// Summary is the small, immutable projection used by the history list. It
// never carries full messages, tool arguments or all upstream snapshots.
type Summary struct {
	PromptTokens      *int   `json:"prompt_tokens,omitempty"`
	CompletionTokens  *int   `json:"completion_tokens,omitempty"`
	ReasoningTokens   *int   `json:"reasoning_tokens,omitempty"`
	TotalTokens       *int   `json:"total_tokens,omitempty"`
	Source            string `json:"source"`
	Complete          *bool  `json:"complete,omitempty"`
	AccumulatedTokens *int   `json:"upstream_accumulated_tokens,omitempty"`
}

func firstCount(m map[string]any, keys ...string) *int {
	for _, key := range keys {
		if n := countPointer(m[key]); n != nil {
			return n
		}
	}
	return nil
}

func Summarize(usage map[string]any) *Summary {
	if usage == nil {
		return nil
	}
	s := &Summary{PromptTokens: firstCount(usage, "prompt_tokens", "input_tokens"), CompletionTokens: firstCount(usage, "completion_tokens", "output_tokens"), ReasoningTokens: firstCount(usage, "reasoning_tokens"), TotalTokens: firstCount(usage, "total_tokens"), Source: "unknown"}
	for _, key := range []string{"completion_tokens_details", "output_tokens_details"} {
		if m, ok := usage[key].(map[string]any); ok {
			if n := firstCount(m, "reasoning_tokens"); n != nil {
				s.ReasoningTokens = n
			}
		}
	}
	var upstream Snapshot
	switch m := usage["ds2api"].(type) {
	case *Metadata:
		if m != nil {
			s.Source = m.Source
			complete := m.Complete
			s.Complete = &complete
			if m.Upstream != nil {
				upstream = m.Upstream.Clone()
			}
			if m.ReasoningSource == "unknown" {
				s.ReasoningTokens = nil
			}
		}
	case map[string]any:
		if source, ok := m["source"].(string); ok {
			s.Source = source
		}
		if complete, ok := m["complete"].(bool); ok {
			s.Complete = &complete
		}
		upstream = FromMap(m["upstream"])
		if m["reasoning_source"] == "unknown" {
			s.ReasoningTokens = nil
		}
	}
	if len(upstream.Messages) > 0 {
		s.AccumulatedTokens = upstream.Messages[len(upstream.Messages)-1].AccumulatedTokens
	}
	return s
}

// CloneMap isolates nested usage details and metadata from callers of a store.
func CloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		return CloneMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	case Snapshot:
		return v.Clone()
	case *Metadata:
		if v == nil {
			return (*Metadata)(nil)
		}
		out := *v
		if v.Upstream != nil {
			snapshot := v.Upstream.Clone()
			out.Upstream = &snapshot
		}
		return &out
	default:
		return v
	}
}
