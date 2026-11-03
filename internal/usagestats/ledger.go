package usagestats

import (
	"strings"
	"time"
)

// Record 在每次成功（或被中止但已有产出）的请求后累计一条用量记录。
// 即使聊天历史关闭也会生效；at 通常传请求开始时间，用于日期分桶与波峰计价。
// 「记录用量」关闭（settings.Enabled=false）时完全不记录。
func (s *Store) Record(model, callerID string, usage map[string]any, at time.Time) {
	s.RecordAccount(model, callerID, "unknown", "", usage, at)
}

// RecordAccount records one effective call; account identifiers are opaque and
// labels contain only a display name and a masked email/mobile identifier.
func (s *Store) RecordAccount(model, callerID, accountID, accountLabel string, usage map[string]any, at time.Time) {
	if s == nil {
		return
	}
	prompt, completion, reasoning, total := ParseUsage(usage)
	if total <= 0 {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	model, callerID = normalizeModel(model), normalizeCallerID(callerID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.settings.Enabled {
		return
	}

	cost := s.computeCostLocked(model, prompt, completion, at)
	accountID = normalizeCallerID(accountID)
	key := entryKey(at.In(gmt8).Format("2006-01-02"), model, callerID, accountID)
	entry := s.entries[key]
	if entry == nil {
		date := at.In(gmt8).Format("2006-01-02")
		entry = &Entry{Date: date, Model: model, CallerID: callerID, AccountID: accountID}
		s.entries[key] = entry
	}
	entry.PromptTokens += prompt
	entry.CompletionTokens += completion
	entry.ReasoningTokens += reasoning
	entry.TotalTokens += total
	entry.Calls++
	entry.Cost += cost
	updateLastUsed(entry, at.UnixMilli(), accountLabel)

	s.dataChangedLocked()
}

func entryKey(date, model, callerID, accountID string) string {
	return strings.Join([]string{date, model, callerID, accountID}, "\x00")
}

func entryIdentityLess(a, b Entry) bool {
	if a.CallerID != b.CallerID {
		return a.CallerID < b.CallerID
	}
	return a.AccountID < b.AccountID
}

func updateLastUsed(entry *Entry, at int64, label string) {
	if at > 0 && at >= entry.LastUsedAt {
		entry.LastUsedAt = at
		if label = strings.TrimSpace(label); label != "" {
			entry.AccountLabel = label
		}
	}
}

func mergeEntry(target *Entry, entry Entry) {
	target.PromptTokens += entry.PromptTokens
	target.CompletionTokens += entry.CompletionTokens
	target.ReasoningTokens += entry.ReasoningTokens
	target.TotalTokens += entry.TotalTokens
	target.Cost += entry.Cost
	target.Calls += entry.Calls
	updateLastUsed(target, entry.LastUsedAt, entry.AccountLabel)
}

func (s *Store) dataChangedLocked() {
	s.revision++
	s.dirty = true
}

// Backfill 仅在账本为空时把聊天历史里的历史请求一次性灌入，避免重复计数。
func (s *Store) Backfill(items []BackfillItem) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) > 0 {
		return false, nil
	}

	rebuilt := map[string]*Entry{}
	for _, item := range items {
		if item.Total <= 0 {
			continue
		}
		model := normalizeModel(item.Model)
		callerID := normalizeCallerID(item.CallerID)
		ts := item.Timestamp
		if ts <= 0 {
			ts = time.Now().UnixMilli()
		}
		at := time.UnixMilli(ts)
		cost := s.computeCostLocked(model, item.Prompt, item.Completion, at)
		key := entryKey(at.In(gmt8).Format("2006-01-02"), model, callerID, "unknown")
		entry := rebuilt[key]
		if entry == nil {
			date := at.In(gmt8).Format("2006-01-02")
			entry = &Entry{Date: date, Model: model, CallerID: callerID, AccountID: "unknown"}
			rebuilt[key] = entry
		}
		entry.PromptTokens += item.Prompt
		entry.CompletionTokens += item.Completion
		entry.ReasoningTokens += item.Reasoning
		entry.TotalTokens += item.Total
		entry.Calls++
		entry.Cost += cost
		updateLastUsed(entry, ts, "")
	}
	if err := s.commitEntriesLocked(rebuilt); err != nil {
		return false, err
	}
	return true, nil
}

func normalizeModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return "unknown"
	}
	return model
}

func normalizeCallerID(callerID string) string {
	callerID = strings.TrimSpace(callerID)
	if callerID == "" {
		return "unknown"
	}
	return callerID
}
