package chathistory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ds2api/internal/util"
)

func (s *Store) rebuildIndexLocked() {
	summaries := make([]SummaryEntry, 0, len(s.details))
	for _, item := range s.details {
		summaries = append(summaries, summaryFromEntry(item))
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].CreatedAt == summaries[j].CreatedAt {
			if summaries[i].Revision == summaries[j].Revision {
				return summaries[i].UpdatedAt > summaries[j].UpdatedAt
			}
			return summaries[i].Revision > summaries[j].Revision
		}
		return summaries[i].CreatedAt > summaries[j].CreatedAt
	})
	if s.state.Limit < DisabledLimit || !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	if s.state.Limit == DisabledLimit {
		s.state.Items = summaries
		return
	}
	if len(summaries) > s.state.Limit {
		keep := make(map[string]struct{}, s.state.Limit)
		for _, item := range summaries[:s.state.Limit] {
			keep[item.ID] = struct{}{}
		}
		for id := range s.details {
			if _, ok := keep[id]; !ok {
				s.markDetailDeletedLocked(id)
				delete(s.details, id)
			}
		}
		summaries = summaries[:s.state.Limit]
	}
	s.state.Items = summaries
}

func (s *Store) nextRevisionLocked() int64 {
	next := time.Now().UnixNano()
	if next <= s.state.Revision {
		next = s.state.Revision + 1
	}
	s.state.Revision = next
	return next
}

func summaryFromEntry(item Entry) SummaryEntry {
	return SummaryEntry{
		ID:             item.ID,
		Revision:       item.Revision,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		CompletedAt:    item.CompletedAt,
		Status:         item.Status,
		CallerID:       item.CallerID,
		AccountID:      item.AccountID,
		Surface:        item.Surface,
		Model:          item.Model,
		Stream:         item.Stream,
		UserInput:      item.UserInput,
		Preview:        buildPreview(item),
		StatusCode:     item.StatusCode,
		ElapsedMs:      item.ElapsedMs,
		FinishReason:   item.FinishReason,
		DetailRevision: item.Revision,
	}
}

func buildPreview(item Entry) string {
	candidate := strings.TrimSpace(item.Content)
	if candidate == "" {
		candidate = strings.TrimSpace(item.ReasoningContent)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(item.Error)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(item.UserInput)
	}
	if truncated, ok := util.TruncateRunes(candidate, defaultPreviewAt); ok {
		return truncated + "..."
	}
	return candidate
}

func ListETag(revision int64) string {
	return fmt.Sprintf(`W/"chat-history-list-%d"`, revision)
}

func DetailETag(id string, revision int64) string {
	return fmt.Sprintf(`W/"chat-history-detail-%s-%d"`, strings.TrimSpace(id), revision)
}
