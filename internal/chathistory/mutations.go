package chathistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Store) Start(params StartParams) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	if s.state.Limit == DisabledLimit {
		return Entry{}, ErrDisabled
	}
	now := time.Now().UnixMilli()
	revision := s.nextRevisionLocked()
	entry := Entry{
		ID:          "chat_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Revision:    revision,
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      "streaming",
		CallerID:    strings.TrimSpace(params.CallerID),
		UserAgent:   strings.TrimSpace(params.UserAgent),
		AccountID:   strings.TrimSpace(params.AccountID),
		Surface:     strings.TrimSpace(params.Surface),
		Model:       strings.TrimSpace(params.Model),
		Stream:      params.Stream,
		UserInput:   strings.TrimSpace(params.UserInput),
		Messages:    cloneMessages(params.Messages),
		Tools:       append(json.RawMessage(nil), params.Tools...),
		ToolChoice:  append(json.RawMessage(nil), params.ToolChoice...),
		ToolMarker:  params.ToolMarker,
		HistoryText: params.HistoryText,
		FinalPrompt: strings.TrimSpace(params.FinalPrompt),
	}
	s.details[entry.ID] = entry
	s.markDetailDirtyLocked(entry.ID)
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return cloneEntry(entry), err
	}
	return cloneEntry(entry), nil
}

func (s *Store) Update(id string, params UpdateParams) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	target := strings.TrimSpace(id)
	if target == "" {
		return Entry{}, errors.New("history id is required")
	}
	item, ok := s.details[target]
	if !ok {
		return Entry{}, errors.New("chat history entry not found")
	}
	now := time.Now().UnixMilli()
	item.Revision = s.nextRevisionLocked()
	item.UpdatedAt = now
	if params.Status != "" {
		item.Status = params.Status
	}
	if params.ReasoningContent != "" || item.ReasoningContent == "" {
		item.ReasoningContent = params.ReasoningContent
	}
	if params.Content != "" || item.Content == "" {
		item.Content = params.Content
	}
	item.Error = strings.TrimSpace(params.Error)
	item.StatusCode = params.StatusCode
	item.ElapsedMs = params.ElapsedMs
	item.FinishReason = strings.TrimSpace(params.FinishReason)
	if params.Usage != nil {
		item.Usage = cloneMap(params.Usage)
	}
	if params.Completed {
		item.CompletedAt = now
	}
	s.details[target] = item
	s.markDetailDirtyLocked(target)
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return Entry{}, err
	}
	return cloneEntry(item), nil
}

func (s *Store) Delete(id string) error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	target := strings.TrimSpace(id)
	if target == "" {
		return errors.New("history id is required")
	}
	if _, ok := s.details[target]; !ok {
		return errors.New("chat history entry not found")
	}
	s.markDetailDeletedLocked(target)
	delete(s.details, target)
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Clear() error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	for id := range s.details {
		s.markDetailDeletedLocked(id)
	}
	s.details = map[string]Entry{}
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) SetLimit(limit int) (File, error) {
	if s == nil {
		return File{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return File{}, s.err
	}
	if !isAllowedLimit(limit) {
		return File{}, fmt.Errorf("unsupported chat history limit: %d", limit)
	}
	s.state.Limit = limit
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return File{}, err
	}
	return cloneFile(s.state), nil
}
