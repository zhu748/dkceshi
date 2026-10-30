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
}

func NewToolCallLinker(prefix string) *ToolCallLinker {
	return &ToolCallLinker{prefix: prefix, reserved: map[string]bool{}, names: map[string]string{}, pending: map[string][]string{}}
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
	s.Reserve(id)
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
	queue := s.pending[linkedName]
	for i, candidate := range queue {
		if candidate == id {
			s.pending[linkedName] = append(queue[:i], queue[i+1:]...)
			break
		}
	}
	return id, name
}
