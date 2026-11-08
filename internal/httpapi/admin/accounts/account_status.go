package accounts

import "ds2api/internal/config"

func accountStatusCounts(accounts []config.Account) map[string]int {
	enabled := 0
	disabled := 0
	muted := 0
	banned := 0
	cooling := 0
	for _, acc := range accounts {
		if acc.IsEnabled() {
			enabled++
		} else {
			disabled++
		}
		if acc.IsMuted() {
			muted++
		}
		if acc.IsBanned() {
			banned++
		}
		if acc.IsCoolingDown() {
			cooling++
		}
	}
	return map[string]int{
		"total":        len(accounts),
		"enabled":      enabled,
		"disabled":     disabled,
		"muted":        muted,
		"banned":       banned,
		"cooling_down": cooling,
	}
}

func matchesAccountStatusFilter(acc config.Account, status string) bool {
	switch status {
	case "enabled":
		return acc.IsEnabled()
	case "disabled":
		return !acc.IsEnabled()
	case "muted":
		return acc.IsMuted()
	case "banned":
		return acc.IsBanned()
	case "cooling_down":
		return acc.IsCoolingDown()
	default:
		return true
	}
}
