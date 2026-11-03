package client

import "strings"

func extractUploadFileResult(resp map[string]any) *UploadFileResult {
	result := &UploadFileResult{Status: "uploaded"}
	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	searchMaps := []map[string]any{resp, data, bizData}
	for _, parent := range []map[string]any{resp, data, bizData} {
		if parent == nil {
			continue
		}
		for _, key := range []string{"file", "biz_data", "data"} {
			if nested, ok := parent[key].(map[string]any); ok {
				searchMaps = append(searchMaps, nested)
			}
		}
	}
	for _, m := range searchMaps {
		if m == nil {
			continue
		}
		readFileTokenUsage(result, m)
		if result.ID == "" {
			result.ID = firstNonEmptyString(m, "id", "file_id")
		}
		if result.Filename == "" {
			result.Filename = firstNonEmptyString(m, "name", "filename", "file_name")
		}
		if result.Status == "uploaded" {
			if status := firstNonEmptyString(m, "status", "file_status"); status != "" {
				result.Status = status
			}
		}
		if !result.IsImage {
			result.IsImage = firstBool(m, "is_image", "isImage")
		}
		if result.Purpose == "" {
			result.Purpose = firstNonEmptyString(m, "purpose")
		}
		if result.AccountID == "" {
			result.AccountID = firstNonEmptyString(m, "account_id", "accountId", "owner_account_id", "ownerAccountId")
		}
		if result.Bytes == 0 {
			result.Bytes = firstPositiveInt64(m, "bytes", "size", "file_size")
		}
	}
	return result
}

func firstBool(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch v := m[key].(type) {
		case bool:
			return v
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "1", "yes", "y":
				return true
			}
		}
	}
	return false
}

func firstNonEmptyString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, _ := m[key].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
