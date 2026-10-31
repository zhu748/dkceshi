package chathistory

import (
	"errors"
	"strings"
	"sync"
)

// Store serializes history updates, revision changes and disk writes under mu.
// The Locked helpers in load, persistence, index and detail_changes require it.
type Store struct {
	mu        sync.Mutex
	path      string
	detailDir string
	state     File
	details   map[string]Entry
	dirty     map[string]struct{}
	deleted   map[string]struct{}
	err       error
}

func New(path string) *Store {
	s := &Store{
		path:      strings.TrimSpace(path),
		detailDir: strings.TrimSpace(path) + ".d",
		state: File{
			Version:  FileVersion,
			Limit:    DefaultLimit,
			Revision: 0,
			Items:    []SummaryEntry{},
		},
		details: map[string]Entry{},
		dirty:   map[string]struct{}{},
		deleted: map[string]struct{}{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = s.loadLocked()
	return s
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) DetailDir() string {
	if s == nil {
		return ""
	}
	return s.detailDir
}

func (s *Store) Err() error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Store) Snapshot() (File, error) {
	if s == nil {
		return File{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return File{}, s.err
	}
	return cloneFile(s.state), nil
}

func (s *Store) Revision() (int64, error) {
	if s == nil {
		return 0, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	return s.state.Revision, nil
}

func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false
	}
	return s.state.Limit != DisabledLimit
}

func (s *Store) Get(id string) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	item, ok := s.details[strings.TrimSpace(id)]
	if !ok {
		return Entry{}, errors.New("chat history entry not found")
	}
	return cloneEntry(item), nil
}

func (s *Store) DetailRevision(id string) (int64, error) {
	if s == nil {
		return 0, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	item, ok := s.details[strings.TrimSpace(id)]
	if !ok {
		return 0, errors.New("chat history entry not found")
	}
	return item.Revision, nil
}
