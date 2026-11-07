package tooldiagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

func New(path string) *Store {
	s := &Store{path: path, state: Index{Version: 1, Settings: Settings{Enabled: true, Limit: 20}, Items: []Summary{}}}
	defer s.publishSettingsLocked()
	defer s.publishSnapshotLocked()
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
		// A stale repair configuration must not make diagnostic evidence and
		// unrelated capture settings read-only. Keep it available for editing,
		// while the runtime explicitly suspends automatic repair.
		s.state.Settings.RepairError = repairErr.Error()
	} else {
		s.state.Settings.Repair, s.state.Settings.RepairError = repair, ""
	}
	// A process cannot resume old requests. Preserve their last durable stage.
	items := append([]Summary(nil), s.state.Items...)
	for i := range items {
		item := &items[i]
		if item.CompletedAt != 0 {
			continue
		}
		record, readErr := s.getLocked(item.ID)
		if readErr != nil {
			s.lastError = readErr
			continue
		}
		if recoverWrittenResponse(&record) {
			record.Revision++
			if writeErr := s.Put(record); writeErr != nil {
				s.lastError = writeErr
			}
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
		if writeErr := s.Put(record); writeErr != nil {
			s.lastError = writeErr
			continue
		}
	}
	if err := s.saveIndexLocked(); err != nil {
		s.lastError = err
	} else {
		s.cleanupDetailVersions()
	}
	return s
}
