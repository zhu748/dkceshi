package promptcompat

import "fmt"

type ContextMessageTransport struct {
	SourceIndex   int    `json:"source_index"`
	LiveIndex     *int   `json:"live_index,omitempty"`
	HistoryIndex  *int   `json:"history_index,omitempty"`
	OmittedReason string `json:"omitted_reason,omitempty"`
}

// Match occurrences, not just membership: identical repeated messages still
// need separate destinations. Renderer fingerprints include message identity
// and extension context as well as the body.
func messageContextTransports(source, live, history []ContextMessageDigest) []ContextMessageTransport {
	index := func(messages []ContextMessageDigest) map[ContextDigest][]int {
		out := map[ContextDigest][]int{}
		for _, message := range messages {
			if message.OmittedReason == "" {
				out[message.NormalizedMessage] = append(out[message.NormalizedMessage], message.Index)
			}
		}
		return out
	}
	liveIndices, historyIndices := index(live), index(history)
	remaining := map[ContextDigest]int{}
	for _, message := range source {
		if message.OmittedReason == "" {
			remaining[message.NormalizedMessage]++
		}
	}
	take := func(indices map[ContextDigest][]int, key ContextDigest) *int {
		if values := indices[key]; len(values) > 0 {
			value := values[0]
			indices[key] = values[1:]
			return &value
		}
		return nil
	}
	out := make([]ContextMessageTransport, 0, len(source))
	for _, message := range source {
		route := ContextMessageTransport{SourceIndex: message.Index, OmittedReason: message.OmittedReason}
		if route.OmittedReason == "" {
			available := len(liveIndices[message.NormalizedMessage]) + len(historyIndices[message.NormalizedMessage])
			route.HistoryIndex = take(historyIndices, message.NormalizedMessage)
			if route.HistoryIndex == nil || available > remaining[message.NormalizedMessage] {
				route.LiveIndex = take(liveIndices, message.NormalizedMessage)
			}
			remaining[message.NormalizedMessage]--
			if route.LiveIndex == nil && route.HistoryIndex == nil {
				route.OmittedReason = "message_has_no_transport_destination"
			}
		}
		out = append(out, route)
	}
	return out
}

// Check the complete standard message objects before publishing a history
// split. This guards message/metadata loss; individual serializers and binary
// omissions are described separately by the manifest.
func ValidateMessageTransportCoverage(source, history, live []any) error {
	counts := map[ContextDigest]int{}
	for _, messages := range [][]any{history, live} {
		for _, raw := range NormalizeLegacyToolHistory(messages) {
			digest, err := digestJSONContext(raw)
			if err != nil {
				return fmt.Errorf("cannot fingerprint message transport: %w", err)
			}
			counts[digest]++
		}
	}
	for _, raw := range NormalizeLegacyToolHistory(source) {
		digest, err := digestJSONContext(raw)
		if err != nil {
			return fmt.Errorf("cannot fingerprint source message: %w", err)
		}
		if counts[digest] == 0 {
			return fmt.Errorf("conversation context was lost while splitting message transport")
		}
		counts[digest]--
	}
	return nil
}
