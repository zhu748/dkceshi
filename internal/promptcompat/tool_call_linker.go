package promptcompat

import (
	"fmt"
	"strings"
)

// ToolCallLinker preserves explicit IDs and matches ID-less compatibility
// results to the oldest unmatched call of the same exact tool name. It is
// request-local; completed calls never become candidates for a later result.
type ToolCallLinker struct {
	prefix   string
	sequence int
	reserved map[string]bool
	names    map[string]string
	pending  map[string][]string
	active   map[string]bool
	err      error
}

func NewToolCallLinker(prefix string) *ToolCallLinker {
	return &ToolCallLinker{prefix: prefix, reserved: map[string]bool{}, names: map[string]string{}, pending: map[string][]string{}, active: map[string]bool{}}
}

// Reserve must be called for explicit IDs before allocating synthetic IDs.
func (s *ToolCallLinker) Reserve(id string) {
	if id = strings.TrimSpace(id); id != "" {
		s.reserved[id] = true
	}
}

func (s *ToolCallLinker) nextID() string {
	for {
		s.sequence++
		id := fmt.Sprintf("%s_%d", s.prefix, s.sequence)
		if !s.reserved[id] {
			s.Reserve(id)
			return id
		}
	}
}

func (s *ToolCallLinker) Call(name, id string) string {
	name, id = strings.TrimSpace(name), strings.TrimSpace(id)
	if id == "" {
		id = s.nextID()
	}
	if s.active[id] {
		s.reject("duplicate pending tool_call_id %q", id)
		return id
	}
	s.Reserve(id)
	s.active[id] = true
	s.names[id] = name
	s.pending[name] = append(s.pending[name], id)
	return id
}

func (s *ToolCallLinker) Result(name, id string) (string, string) {
	name, id = strings.TrimSpace(name), strings.TrimSpace(id)
	if id == "" && len(s.pending[name]) > 0 {
		id = s.pending[name][0]
	}
	if id == "" {
		id = s.nextID()
	}
	s.Reserve(id)
	linkedName := s.names[id]
	if name == "" {
		name = linkedName
	}
	if linkedName != "" && name != linkedName {
		s.reject("tool result name does not match tool_call_id %q", id)
		return id, name
	}
	if linkedName != "" && !s.active[id] {
		s.reject("duplicate tool result for tool_call_id %q", id)
		return id, name
	}
	delete(s.active, id)
	queue := s.pending[linkedName]
	for i, candidate := range queue {
		if candidate == id {
			s.pending[linkedName] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	return id, name
}

func (s *ToolCallLinker) reject(format string, values ...any) {
	if s.err == nil {
		s.err = fmt.Errorf(format, values...)
	}
}
func (s *ToolCallLinker) Err() error { return s.err }

// Partial histories may contain results without their original calls. Reject
// only contradictions to calls actually present in this request.
func ValidateToolHistory(messages []any) error {
	links := NewToolCallLinker("history")
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		links.Reserve(asString(message["tool_call_id"]))
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			links.Reserve(asString(call["id"]))
		}
	}

	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			function, _ := call["function"].(map[string]any)
			links.Call(asString(function["name"]), asString(call["id"]))
		}
		role := strings.ToLower(strings.TrimSpace(asString(message["role"])))
		if role == "tool" || role == "function" {
			links.Result(asString(message["name"]), asString(message["tool_call_id"]))
		}
		if links.Err() != nil {
			return links.Err()
		}
	}
	return nil
}
