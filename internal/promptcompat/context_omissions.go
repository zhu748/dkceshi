package promptcompat

// Omissions describe normalized fields without embedding their values. They
// expose missing serializers separately from intentional transport-only hints.
type ContextOmission struct {
	MessageIndex int    `json:"message_index"`
	Field        string `json:"field,omitempty"`
	Reason       string `json:"reason"`
}

func contextOmissions(messages []ContextMessageDigest) []ContextOmission {
	var out []ContextOmission
	for _, message := range messages {
		if message.OmittedReason != "" {
			out = append(out, ContextOmission{MessageIndex: message.Index, Reason: message.OmittedReason})
			continue
		}
		for _, field := range message.Fields {
			if field.OmittedReason != "" {
				out = append(out, ContextOmission{MessageIndex: message.Index, Field: field.Name, Reason: field.OmittedReason})
			}
		}
	}
	return out
}
