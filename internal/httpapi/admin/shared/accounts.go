package shared

import (
	"strings"

	"ds2api/internal/config"
)

func ReverseAccounts(a []config.Account) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func ToAccount(m map[string]any) config.Account {
	email := FieldString(m, "email")
	mobile := config.NormalizeMobileForStorage(FieldString(m, "mobile"))
	acc := config.Account{
		Name:        FieldString(m, "name"),
		Remark:      FieldString(m, "remark"),
		Email:       email,
		Mobile:      mobile,
		Password:    FieldString(m, "password"),
		DeviceModel: FieldString(m, "device_model"), AndroidAPILevel: FieldString(m, "android_api_level"), RangersID: FieldString(m, "rangers_id"),
		DeviceID:       FieldString(m, "device_id"),
		InstallationID: FieldString(m, "installation_id"), AndroidID: FieldString(m, "android_id"),
		DeviceBrand: FieldString(m, "device_brand"), Timezone: FieldString(m, "timezone"),
		PictureCompressFormat: FieldString(m, "picture_compress_format"),
		ProxyID:               FieldString(m, "proxy_id"),
		PoolType:              config.NormalizePoolType(FieldString(m, "pool_type")),
		Priority:              fieldInt(m, "priority"),
		Locale:                FieldString(m, "locale"),
	}
	if ratio, ok := m["photo_compress_ratio"].(float64); ok {
		acc.PhotoCompressRatio = &ratio
	}
	return acc
}

func AccountMatchesIdentifier(acc config.Account, identifier string) bool {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return false
	}
	if strings.TrimSpace(acc.Email) == id {
		return true
	}
	if mobileKey := config.CanonicalMobileKey(id); mobileKey != "" && mobileKey == config.CanonicalMobileKey(acc.Mobile) {
		return true
	}
	return acc.Identifier() == id
}

func NormalizeAccountForStorage(acc config.Account) config.Account {
	acc.Name = strings.TrimSpace(acc.Name)
	acc.Remark = strings.TrimSpace(acc.Remark)
	acc.Email = strings.TrimSpace(acc.Email)
	acc.Mobile = config.NormalizeMobileForStorage(acc.Mobile)
	acc.DeviceID = strings.TrimSpace(acc.DeviceID)
	acc.ProxyID = strings.TrimSpace(acc.ProxyID)
	acc.PoolType = config.NormalizePoolType(acc.PoolType)
	acc.Locale = strings.TrimSpace(acc.Locale)
	return acc
}

func AccountDedupeKey(acc config.Account) string {
	if email := strings.TrimSpace(acc.Email); email != "" {
		return "email:" + email
	}
	if mobile := config.CanonicalMobileKey(acc.Mobile); mobile != "" {
		return "mobile:" + mobile
	}
	if id := strings.TrimSpace(acc.Identifier()); id != "" {
		return "id:" + id
	}
	return ""
}

func NormalizeAndDedupeAccounts(accounts []config.Account) []config.Account {
	if len(accounts) == 0 {
		return nil
	}
	out := make([]config.Account, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, acc := range accounts {
		acc = NormalizeAccountForStorage(acc)
		key := AccountDedupeKey(acc)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, acc)
	}
	return out
}

func FindAccountByIdentifier(store ConfigStore, identifier string) (config.Account, bool) {
	id := strings.TrimSpace(identifier)
	if id == "" {
		return config.Account{}, false
	}
	if acc, ok := store.FindAccount(id); ok {
		return acc, true
	}
	accounts := store.Snapshot().Accounts
	for _, acc := range accounts {
		if AccountMatchesIdentifier(acc, id) {
			return acc, true
		}
	}
	return config.Account{}, false
}
