package shared

import (
	"fmt"
	"strings"

	"ds2api/internal/config"
)

func NormalizeAPIKeyForStorage(item config.APIKey) config.APIKey {
	return config.NormalizeAPIKey(item)
}

func ToAPIKeys(v any) ([]config.APIKey, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]config.APIKey, 0, len(arr))
	seen := map[string]struct{}{}
	for _, item := range arr {
		switch x := item.(type) {
		case map[string]any:
			key := FieldString(x, "key")
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, apiKeyFromMap(x))
		default:
			key := strings.TrimSpace(fmt.Sprintf("%v", item))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, config.APIKey{Key: key})
		}
	}
	return out, true
}

// apiKeyFromMap builds one API key from a decoded request object. The typed
// fields are read by name; the boolean switches come from the credential-flag
// registry, so a new per-key flag needs no change here.
func apiKeyFromMap(m map[string]any) config.APIKey {
	item := config.APIKey{
		Key:    FieldString(m, "key"),
		Name:   FieldString(m, "name"),
		Remark: FieldString(m, "remark"),
	}
	ApplyCredentialFlags(&item, m)
	return item
}

// ApplyCredentialFlags copies every registered credential flag supplied by m
// onto item. A flag that is absent, or that carries an explicit JSON null, is
// "not supplied": it keeps the value already on item, so a partial update never
// clears a switch the request did not actually set. Both readings come from
// FieldBoolOptional and the registry, so a newly registered flag inherits the
// rule without any change here.
func ApplyCredentialFlags(item *config.APIKey, m map[string]any) {
	for _, spec := range config.CredentialFlagSpecs() {
		value, ok := FieldBoolOptional(m, spec.Name)
		if !ok {
			continue
		}
		item.SetFlag(spec.Name, value)
	}
}

func APIKeyHasMetadata(item config.APIKey) bool {
	return strings.TrimSpace(item.Name) != "" || strings.TrimSpace(item.Remark) != ""
}

func MergeAPIKeysPreferStructured(existing, incoming []config.APIKey) ([]config.APIKey, int) {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil, 0
	}

	merged := make([]config.APIKey, 0, len(existing)+len(incoming))
	index := make(map[string]int, len(existing)+len(incoming))
	for _, item := range existing {
		item = config.NormalizeAPIKey(item)
		if item.Key == "" {
			continue
		}
		if _, ok := index[item.Key]; ok {
			continue
		}
		index[item.Key] = len(merged)
		merged = append(merged, item)
	}

	imported := 0
	for _, item := range incoming {
		item = config.NormalizeAPIKey(item)
		if item.Key == "" {
			continue
		}
		if idx, ok := index[item.Key]; ok {
			keep := merged[idx]
			next := MergeAPIKeyRecord(keep, item)
			if !config.APIKeyEqual(next, keep) {
				merged[idx] = next
				imported++
			}
			continue
		}
		index[item.Key] = len(merged)
		merged = append(merged, item)
		imported++
	}

	if len(merged) == 0 {
		return nil, imported
	}
	return merged, imported
}

func MergeAPIKeyRecord(existing, incoming config.APIKey) config.APIKey {
	existing = config.NormalizeAPIKey(existing)
	incoming = config.NormalizeAPIKey(incoming)
	if existing.Key == "" {
		return incoming
	}
	var keep config.APIKey
	if APIKeyHasMetadata(existing) {
		keep = existing
	} else if APIKeyHasMetadata(incoming) {
		keep = incoming
	} else {
		keep = existing
	}
	// 各凭据开关的合并策略由注册表声明（限制性开关两侧取或，避免配置导入
	// 静默丢掉用户已显式设置的限制）。
	return config.MergeCredentialFlags(keep, incoming)
}
