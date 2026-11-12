package tokenusage

import "strings"

// Observation records counts for one assistant message. AccumulatedTokens has
// includes context; it is never assumed to be output. Paired history counts
// provide explicit request input/output/total through Snapshot.Request.
type Observation struct {
	MessageID         int  `json:"message_id,omitempty"`
	InputTokens       *int `json:"input_tokens,omitempty"`
	OutputTokens      *int `json:"output_tokens,omitempty"`
	ReasoningTokens   *int `json:"reasoning_tokens,omitempty"`
	TotalTokens       *int `json:"total_tokens,omitempty"`
	AccumulatedTokens *int `json:"accumulated_tokens,omitempty"`
}

func (o Observation) Empty() bool {
	return o.InputTokens == nil && o.OutputTokens == nil && o.ReasoningTokens == nil && o.TotalTokens == nil && o.AccumulatedTokens == nil
}

// Parse only visits response-level metadata paths. Tool JSON, file token_usage,
// search results and fragment-local counters cannot become request usage.
func Parse(chunk map[string]any) Observation {
	var o Observation
	var visit func(any, string, int)
	visit = func(value any, path string, depth int) {
		if depth > 16 {
			return
		}
		if values, ok := value.([]any); ok {
			for _, child := range values {
				visit(child, path, depth+1)
			}
			return
		}
		m, ok := value.(map[string]any)
		if !ok {
			return
		}
		if p, ok := m["p"].(string); ok {
			p = strings.Trim(p, "/")
			if strings.HasPrefix(p, "response/") || p == "response" {
				path = p
			} else if path != "" {
				path += "/" + p
			} else {
				path = p
			}
			if strings.Contains(path, "fragments") {
				return
			}
			if path == "response" || path == "" || path == "response/usage" || path == "response/token_usage" || path == "usage" || path == "token_usage" {
				visit(m["v"], path, depth+1)
			} else {
				o.field(strings.TrimPrefix(path, "response/"), m["v"])
			}
			return
		}
		if path != "" && path != "response" && path != "usage" && path != "token_usage" && path != "response/usage" && path != "response/token_usage" {
			return
		}
		for key, val := range m {
			switch key {
			case "response":
				visit(val, "response", depth+1)
			case "v":
				visit(val, path, depth+1)
			case "usage", "token_usage":
				visit(val, key, depth+1)
			default:
				o.field(key, val)
			}
		}
	}
	visit(chunk, "", 0)
	return o
}

func (o *Observation) field(key string, value any) {
	key = strings.TrimPrefix(strings.TrimPrefix(key, "usage/"), "token_usage/")
	switch key {
	case "message_id", "response_message_id":
		if n, ok := Count(value); ok {
			o.MessageID = n
		}
	case "input_tokens", "prompt_tokens":
		if n := countPointer(value); n != nil {
			o.InputTokens = n
		}
	case "output_tokens", "completion_tokens":
		if n := countPointer(value); n != nil {
			o.OutputTokens = n
		}
	case "reasoning_tokens", "completion_tokens_details/reasoning_tokens", "output_tokens_details/reasoning_tokens":
		if n := countPointer(value); n != nil {
			o.ReasoningTokens = n
		}
	case "total_tokens":
		if n := countPointer(value); n != nil {
			o.TotalTokens = n
		}
	case "accumulated_token_usage":
		if n := countPointer(value); n != nil {
			o.AccumulatedTokens = n
		}
	case "completion_tokens_details", "output_tokens_details":
		if m, ok := value.(map[string]any); ok {
			o.ReasoningTokens = countPointer(m["reasoning_tokens"])
		}
	}
}

func (o Observation) Clone() Observation {
	var snapshot Snapshot
	snapshot.Observe(o)
	if len(snapshot.Messages) == 0 {
		return o
	}
	return snapshot.Messages[0]
}
