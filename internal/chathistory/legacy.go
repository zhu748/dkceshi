package chathistory

import (
	"encoding/json"
	"strings"
	"time"
)

type legacyFile struct {
	Version int     `json:"version"`
	Limit   int     `json:"limit"`
	Items   []Entry `json:"items"`
}

type legacyProbe struct {
	Items []map[string]json.RawMessage `json:"items"`
}

func (s *Store) loadLegacyLocked(legacy legacyFile) {
	s.state.Version = FileVersion
	s.state.Limit = legacy.Limit
	if !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	s.details = map[string]Entry{}
	s.dirty = map[string]struct{}{}
	s.deleted = map[string]struct{}{}
	maxRevision := int64(0)
	for _, item := range legacy.Items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		item.Messages = cloneMessages(item.Messages)
		if item.Revision == 0 {
			if item.UpdatedAt > 0 {
				item.Revision = item.UpdatedAt
			} else {
				item.Revision = time.Now().UnixNano()
			}
		}
		if item.Revision > maxRevision {
			maxRevision = item.Revision
		}
		s.details[item.ID] = item
		s.markDetailDirtyLocked(item.ID)
	}
	s.state.Revision = maxRevision
	s.rebuildIndexLocked()
}

func parseLegacy(raw []byte) (legacyFile, bool, error) {
	var legacy legacyFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return legacyFile{}, false, nil
	}
	if len(legacy.Items) == 0 {
		return legacy, false, nil
	}
	var probe legacyProbe
	if err := json.Unmarshal(raw, &probe); err == nil {
		for _, item := range probe.Items {
			if _, ok := item["detail_revision"]; ok {
				return legacy, false, nil
			}
		}
	}
	return legacy, true, nil
}
