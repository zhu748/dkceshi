package accounts

import (
	"fmt"
	"math"
	"strings"

	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

func validateImportedAccount(acc config.Account) error {
	if acc.Identifier() == "" {
		return fmt.Errorf("需要 email 或 mobile")
	}
	switch strings.ToLower(strings.TrimSpace(acc.PoolType)) {
	case "", config.PoolTypeDefault, config.PoolTypeNoTools, config.PoolTypeToolsOnly:
	default:
		return fmt.Errorf("pool_type 不受支持")
	}
	for name, value := range map[string]float64{"muted_until": acc.MutedUntil, "cooldown_until": acc.CooldownUntil, "node_cooldown_until": acc.NodeCooldownUntil} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 253402300799 {
			return fmt.Errorf("%s 必须为有效的非负 Unix 秒时间戳", name)
		}
	}
	if acc.HIFDliqExpiresAt < 0 || acc.HIFLeimExpiresAt < 0 {
		return fmt.Errorf("HIF 到期时间不能为负数")
	}
	return config.ValidateAccountDeviceProfile(acc)
}

func accountIdentityKeys(acc config.Account) []string {
	var keys []string
	if email := strings.TrimSpace(acc.Email); email != "" {
		keys = append(keys, "email:"+strings.ToLower(email))
	}
	if mobile := config.CanonicalMobileKey(acc.Mobile); mobile != "" {
		keys = append(keys, "mobile:"+mobile)
	}
	return keys
}

// Upsert complete records so omitted false/zero values in our JSON backup also
// restore switches and clear expired states. Unmentioned accounts stay intact.
func mergeImportedAccounts(existing, incoming []config.Account) ([]config.Account, int, int, error) {
	out := append([]config.Account(nil), existing...)
	index := make(map[string]int, len(existing)*2)
	for i, acc := range existing {
		for _, key := range accountIdentityKeys(acc) {
			if _, duplicate := index[key]; duplicate {
				return nil, 0, 0, fmt.Errorf("现有账号标识重复，请先整理后导入")
			}
			index[key] = i
		}
	}
	seen := make(map[string]bool, len(incoming)*2)
	updatedTargets := make(map[int]bool)
	added, updated := 0, 0
	for i, raw := range incoming {
		acc := adminshared.NormalizeAccountForStorage(raw)
		keys := accountIdentityKeys(acc)
		target := -1
		for _, key := range keys {
			if seen[key] {
				return nil, 0, 0, fmt.Errorf("第 %d 条账号与文件中的其他账号标识重复", i+1)
			}
			if match, exists := index[key]; exists {
				if target >= 0 && target != match {
					return nil, 0, 0, fmt.Errorf("第 %d 条账号的邮箱和手机号分别属于不同账号", i+1)
				}
				target = match
			}
		}
		if target >= 0 {
			if updatedTargets[target] {
				return nil, 0, 0, fmt.Errorf("第 %d 条账号重复引用了同一个现有账号", i+1)
			}
			updatedTargets[target] = true
			out[target] = acc
			updated++
		} else {
			out = append(out, acc)
			added++
		}
		for _, key := range keys {
			seen[key] = true
		}
	}
	return out, added, updated, nil
}
