package usagestats

import (
	"encoding/json"
	"strings"
)

// legacyModelUsage 与 legacyDayUsage 是 v1（按天+模型聚合）账本的旧结构，
// 仅在加载时用于迁移，不再写出。
type legacyModelUsage struct {
	Requests     int64 `json:"requests"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type legacyDayUsage struct {
	Date   string                      `json:"date"`
	Models map[string]legacyModelUsage `json:"models"`
}

type legacyV1File struct {
	Version int                        `json:"version"`
	Days    map[string]*legacyDayUsage `json:"days"`
}

// decodeLegacyArray 兼容旧版（无版本包装的纯 Entry 数组）文件。
func decodeLegacyArray(raw []byte) ([]Entry, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	var list []Entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false
	}
	return list, true
}

// decodeLegacyV1Days 识别 v1 按天+模型聚合的旧账本结构。
func decodeLegacyV1Days(raw []byte) (map[string]*legacyDayUsage, bool) {
	var v1 legacyV1File
	if err := json.Unmarshal(raw, &v1); err != nil || len(v1.Days) == 0 {
		return nil, false
	}
	return v1.Days, true
}

func migrateLegacyV1Days(days map[string]*legacyDayUsage) map[string]*Entry {
	entries := map[string]*Entry{}
	for date, day := range days {
		if day == nil {
			continue
		}
		for model, usage := range day.Models {
			normalized := normalizeModel(model)
			key := entryKey(date, normalized, "unknown", "unknown")
			entry := entries[key]
			if entry == nil {
				entry = &Entry{Date: date, Model: normalized, CallerID: "unknown", AccountID: "unknown"}
				entries[key] = entry
			}
			entry.PromptTokens += usage.InputTokens
			entry.CompletionTokens += usage.OutputTokens
			entry.TotalTokens += usage.InputTokens + usage.OutputTokens
			entry.Calls += usage.Requests
		}
	}
	return entries
}

func (s *Store) ingestEntries(list []Entry) {
	for _, entry := range list {
		entry.Model = normalizeModel(entry.Model)
		entry.CallerID = normalizeCallerID(entry.CallerID)
		entry.AccountID = normalizeCallerID(entry.AccountID)
		key := entryKey(entry.Date, entry.Model, entry.CallerID, entry.AccountID)
		if existing := s.entries[key]; existing != nil {
			mergeEntry(existing, entry)
		} else {
			copied := entry
			s.entries[key] = &copied
		}
	}
}
