package tokenusage

// Snapshot is bounded metadata for the current generation. Repeated snapshots
// of the same message replace counters rather than adding cumulative values.
type Snapshot struct {
	Request   *Observation  `json:"request,omitempty"`
	Messages  []Observation `json:"messages,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
}

const MaxMessages = 64

func (s *Snapshot) Observe(o Observation) {
	if o.Empty() && o.MessageID == 0 {
		return
	}
	i := len(s.Messages) - 1
	if o.MessageID > 0 {
		for i >= 0 && s.Messages[i].MessageID != o.MessageID && s.Messages[i].MessageID != 0 {
			i--
		}
	}
	if i < 0 {
		if len(s.Messages) >= MaxMessages {
			s.Truncated = true
			return
		}
		s.Messages = append(s.Messages, Observation{})
		i = len(s.Messages) - 1
	}
	if i < len(s.Messages)-1 {
		current := s.Messages[i]
		copy(s.Messages[i:], s.Messages[i+1:])
		s.Messages[len(s.Messages)-1] = current
		i = len(s.Messages) - 1
	}
	target := &s.Messages[i]
	if o.MessageID > 0 {
		target.MessageID = o.MessageID
	}
	merge := func(dst **int, src *int) {
		if src != nil && (*dst == nil || *src >= **dst) {
			n := *src
			*dst = &n
		}
	}
	merge(&target.InputTokens, o.InputTokens)
	merge(&target.OutputTokens, o.OutputTokens)
	merge(&target.ReasoningTokens, o.ReasoningTokens)
	merge(&target.TotalTokens, o.TotalTokens)
	merge(&target.AccumulatedTokens, o.AccumulatedTokens)
}

func (s *Snapshot) Merge(other Snapshot) {
	if other.Request != nil {
		s.SetRequest(*other.Request)
	}
	for _, o := range other.Messages {
		s.Observe(o)
	}
	s.Truncated = s.Truncated || other.Truncated
}

func (s Snapshot) Clone() Snapshot {
	var result Snapshot
	result.Merge(s)
	return result
}

// FromMap validates bridge metadata instead of trusting client supplied counts.
func FromMap(value any) Snapshot {
	if snapshot, ok := value.(Snapshot); ok {
		return snapshot.Clone()
	}
	var s Snapshot
	m, _ := value.(map[string]any)
	if req, ok := m["request"].(map[string]any); ok {
		observed := Parse(map[string]any{"usage": req})
		observed.MessageID, _ = Count(req["message_id"])
		s.SetRequest(observed)
	}
	items, _ := m["messages"].([]any)
	if len(items) > MaxMessages {
		s.Truncated = true
		items = items[:MaxMessages]
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		o := Observation{InputTokens: countPointer(m["input_tokens"]), OutputTokens: countPointer(m["output_tokens"]), ReasoningTokens: countPointer(m["reasoning_tokens"]), TotalTokens: countPointer(m["total_tokens"]), AccumulatedTokens: countPointer(m["accumulated_tokens"])}
		o.MessageID, _ = Count(m["message_id"])
		s.Observe(o)
	}
	s.Truncated = s.Truncated || m["truncated"] == true
	return s
}

func (s *Snapshot) SetRequest(observed Observation) {
	var copy Snapshot
	copy.Observe(observed)
	if len(copy.Messages) > 0 {
		o := copy.Messages[0]
		s.Request = &o
	}
}
func (s Snapshot) Effective() (Observation, bool) {
	if s.Request != nil {
		return *s.Request, true
	}
	if len(s.Messages) == 1 && !s.Truncated {
		return s.Messages[0], true
	}
	return Observation{}, false
}
