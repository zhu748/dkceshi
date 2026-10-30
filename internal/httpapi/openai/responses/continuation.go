package responses

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/promptcompat"
)

var errContinuationUnauthorized = errors.New("unauthorized")

// restoreContinuation runs before uploads and account acquisition. Store records
// contain caller-owned replay context, never remote session IDs or credentials.
func (h *Handler) restoreContinuation(r *http.Request, req map[string]any) ([]any, error) {
	if value, exists := req["store"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("store must be a boolean")
		}
	}
	if req["conversation"] != nil {
		return nil, fmt.Errorf("conversation is not supported; use previous_response_id or full input")
	}
	var previous []any
	if raw := req["previous_response_id"]; raw != nil {
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("previous_response_id must be a non-empty string")
		}
		caller, err := h.Auth.DetermineCaller(r)
		if err != nil {
			return nil, errContinuationUnauthorized
		}
		var found bool
		previous, found = h.getResponseStore().context(responseStoreOwner(caller), id)
		if !found {
			return nil, fmt.Errorf("previous_response_id was not found, expired, or belongs to another caller")
		}
	}
	// Top-level instructions apply to this response only. Explicit messages in
	// input remain part of the replayable conversation.
	current := promptcompat.ResponsesMessagesFromRequest(map[string]any{"input": req["input"], "messages": req["messages"]})
	if len(current) == 0 {
		return nil, fmt.Errorf("request must include non-empty input or messages")
	}
	messages := append(previous, current...)
	names := map[string]string{}
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		calls, _ := msg["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			fn, _ := call["function"].(map[string]any)
			if id, _ := call["id"].(string); id != "" {
				names[id], _ = fn["name"].(string)
			}
		}
		if msg["role"] == "tool" && msg["name"] == nil {
			if id, _ := msg["tool_call_id"].(string); names[id] != "" {
				msg["name"] = names[id]
			}
		}
	}
	req["messages"] = messages
	delete(req, "input")
	return cloneMessages(messages), nil
}

func (h *Handler) responsePersister(owner, id string, req map[string]any, context []any) func(map[string]any) {
	enabled := req["store"] != false
	return func(value map[string]any) {
		value["store"] = enabled
		value["previous_response_id"] = req["previous_response_id"]
		if !enabled {
			return
		}
		output, _ := value["output"].([]any)
		// Stream renderers may use typed slices; cloneValue canonicalizes them.
		if output == nil {
			output, _ = cloneValue(value["output"]).([]any)
		}
		replay := append(cloneMessages(context), promptcompat.NormalizeResponsesInputAsMessages(output)...)
		h.getResponseStore().putContext(owner, id, value, replay)
	}
}
