package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

func New(path string) *Store {
	s := &Store{path: path, state: Index{Version: 1, Settings: Settings{Enabled: true, Limit: 20}, Items: []Summary{}}}
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &s.state)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.lastError, s.loadError = err, err
		return s
	}
	if s.state.Settings.Limit != 20 && s.state.Settings.Limit != 50 {
		s.state.Settings.Limit = 20
	}
	repair, repairErr := mergeRepairSettings(nil, s.state.Settings.Repair)
	if repairErr != nil {
		s.lastError, s.loadError = repairErr, repairErr
		return s
	}
	s.state.Settings.Repair = repair
	// A process cannot resume old requests. Preserve their last durable stage.
	for i := range s.state.Items {
		item := &s.state.Items[i]
		if item.CompletedAt != 0 {
			continue
		}
		record, readErr := s.getLocked(item.ID)
		if readErr != nil {
			s.lastError = readErr
			continue
		}
		record.Status, record.Pinned, record.Delivery = "interrupted", true, "not_delivered"
		record.ErrorCode, record.Error = "process_interrupted", "Process stopped before diagnostics completed."
		record.CompletedAt, record.UpdatedAt = time.Now().UnixMilli(), time.Now().UnixMilli()
		record.Revision++
		if len(record.Attempts) > 0 {
			a := &record.Attempts[len(record.Attempts)-1]
			a.Result = "interrupted"
			a.ErrorCode, a.Error = record.ErrorCode, record.Error
			a.CompletedAt = record.CompletedAt
		}
		if writeErr := atomicJSON(s.detailPath(record.ID), record); writeErr != nil {
			s.lastError = writeErr
			continue
		}
		*item = record.Summary
	}
	if err := s.saveIndexLocked(); err != nil {
		s.lastError = err
	}
	return s
}
