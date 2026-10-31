package toolcall

import (
	"ds2api/internal/jsonvalue"
	"encoding/json"
	"html"
	"strings"
	"unicode"
)

func parseToolCallInput(v any) map[string]any {
	switch x := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return x
	case string:
		raw := strings.TrimSpace(html.UnescapeString(x))
		if raw == "" {
			return map[string]any{}
		}
		var parsed map[string]any
		if err := jsonvalue.Unmarshal([]byte(raw), &parsed); err == nil && parsed != nil {
			repairPathLikeControlChars(parsed)
			return parsed
		}
		// Try to repair invalid backslashes (common in Windows paths output by models)
		repaired := repairInvalidJSONBackslashes(raw)
		if repaired != raw {
			if err := jsonvalue.Unmarshal([]byte(repaired), &parsed); err == nil && parsed != nil {
				repairPathLikeControlChars(parsed)
				return parsed
			}
		}
		// Try to repair loose JSON in string argument as well
		repairedLoose := RepairLooseJSON(raw)
		if repairedLoose != raw {
			if err := jsonvalue.Unmarshal([]byte(repairedLoose), &parsed); err == nil && parsed != nil {
				repairPathLikeControlChars(parsed)
				return parsed
			}
		}
		return map[string]any{"_raw": raw}
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return map[string]any{}
		}
		var parsed map[string]any
		if err := jsonvalue.Unmarshal(b, &parsed); err == nil && parsed != nil {
			return parsed
		}
		return map[string]any{}
	}
}

func repairPathLikeControlChars(m map[string]any) {
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			repairPathLikeControlChars(vv)
		case []any:
			for _, item := range vv {
				if child, ok := item.(map[string]any); ok {
					repairPathLikeControlChars(child)
				}
			}
		case string:
			if isPathLikeKey(k) && containsControlRune(vv) {
				m[k] = escapeControlRunes(vv)
			}
		}
	}
}

// isPathLikeKey reports whether a parameter key names a filesystem path or
// file. The match is word-based: plain substring matching also caught keys
// like "profile" (contains "file") or "xpath" / "critical_path" (contain
// "path") and silently rewrote their values by escaping real control runes.
func isPathLikeKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	// Whole compounds that never split into a standalone "path"/"file" word
	// but still name a file location.
	switch lower {
	case "filename", "filenames", "pathname", "filepath", "filepaths":
		return true
	}
	// Split on non-alphanumerics (file_path, "file name") and on camelCase
	// humps (filePath -> file|Path, from the original key). The path/file word
	// must be the FINAL word: in key naming it is the head noun ("file_path",
	// "output_file"), while a leading occurrence is usually descriptive
	// ("path_analysis", "file_hash", "critical_path_study").
	words := splitKeyWords(trimmed)
	if len(words) == 0 {
		return false
	}
	switch words[len(words)-1] {
	case "path", "paths", "file", "files":
		return true
	}
	// Separated compounds whose head noun is not path/file itself
	// (file-name, file_name) collapse back to the explicit list.
	switch strings.Join(words, "") {
	case "filename", "filenames", "pathname", "filepath", "filepaths":
		return true
	}
	return false
}

// splitKeyWords splits a parameter key into lowercase words on
// non-alphanumeric separators and camelCase humps. Keys are short, so the
// double pass is negligible.
func splitKeyWords(key string) []string {
	var out []string
	start := 0
	flush := func(end int) {
		if end > start {
			out = append(out, strings.ToLower(key[start:end]))
		}
	}
	for i, r := range key {
		isUpper := r >= 'A' && r <= 'Z'
		isAlnum := isUpper || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if !isAlnum {
			flush(i)
			start = i + 1
			continue
		}
		if isUpper && i > start {
			flush(i)
			start = i
		}
	}
	flush(len(key))
	return out
}

func containsControlRune(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func escapeControlRunes(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
