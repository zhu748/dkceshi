package usagestats

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var ErrInvalidUsageEntry = errors.New("invalid usage entry")

// Counters are represented as JSON numbers in the browser, so they must remain
// exact there as well as fitting in int64 on the server.
const maxUsageCounter int64 = 1<<53 - 1

func validateEntry(entry Entry) error {
	for _, id := range []string{entry.Model, entry.CallerID, entry.AccountID} {
		if strings.ContainsRune(id, '\x00') {
			return fmt.Errorf("%w: identifiers cannot contain NUL", ErrInvalidUsageEntry)
		}
	}
	for _, n := range []int64{entry.PromptTokens, entry.CompletionTokens, entry.ReasoningTokens, entry.TotalTokens, entry.Calls} {
		if n < 0 || n > maxUsageCounter {
			return fmt.Errorf("%w: counters must be nonnegative safe integers", ErrInvalidUsageEntry)
		}
	}
	if entry.TotalTokens < entry.PromptTokens+entry.CompletionTokens || entry.ReasoningTokens > entry.CompletionTokens {
		return fmt.Errorf("%w: inconsistent token totals", ErrInvalidUsageEntry)
	}
	if math.IsNaN(entry.Cost) || math.IsInf(entry.Cost, 0) || entry.Cost < 0 {
		return fmt.Errorf("%w: cost must be finite and nonnegative", ErrInvalidUsageEntry)
	}
	if entry.LastUsedAt < 0 || entry.LastUsedAt > 8640000000000000 {
		return fmt.Errorf("%w: last_used_at is outside the supported timestamp range", ErrInvalidUsageEntry)
	}
	return nil
}

// Import adds validated entries under the full date/model/caller/account key.
// A malformed numeric row or a disk failure leaves the complete ledger intact.
// Legacy invalid dates/empty models are skipped, retaining the prior API contract.
func (s *Store) Import(entries []Entry) (int, error) {
	if s == nil {
		return 0, nil
	}
	valid := make([]Entry, 0, len(entries))
	for i, entry := range entries {
		entry.Date = strings.TrimSpace(entry.Date)
		if _, err := time.Parse("2006-01-02", entry.Date); err != nil || strings.TrimSpace(entry.Model) == "" {
			continue
		}
		entry.Model = normalizeModel(entry.Model)
		entry.CallerID = normalizeCallerID(entry.CallerID)
		entry.AccountID = normalizeCallerID(entry.AccountID)
		if entry.TotalTokens == 0 && entry.PromptTokens >= 0 && entry.CompletionTokens >= 0 && entry.PromptTokens <= maxUsageCounter && entry.CompletionTokens <= maxUsageCounter {
			entry.TotalTokens = entry.PromptTokens + entry.CompletionTokens
		}
		if err := validateEntry(entry); err != nil {
			return 0, fmt.Errorf("row %d: %w", i+1, err)
		}
		valid = append(valid, entry)
	}
	if len(valid) == 0 {
		return 0, nil
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate := make(map[string]*Entry, len(s.entries)+len(valid))
	for key, entry := range s.entries {
		copied := *entry
		candidate[key] = &copied
	}
	for _, entry := range valid {
		key := entryKey(entry.Date, entry.Model, entry.CallerID, entry.AccountID)
		if existing := candidate[key]; existing != nil {
			mergeEntry(existing, entry)
			if err := validateEntry(*existing); err != nil {
				return 0, err
			}
		} else {
			copied := entry
			candidate[key] = &copied
		}
	}
	if err := s.commitEntriesLocked(candidate); err != nil {
		return 0, err
	}
	return len(valid), nil
}
