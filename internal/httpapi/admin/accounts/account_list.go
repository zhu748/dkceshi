package accounts

import (
	"strings"

	"ds2api/internal/config"
)

// Filter and stably partition indices in O(N). Sorting full Account structs
// repeatedly copies credentials and device profiles; responses need only a page.
// Each caller defines its own eligibility rule and configuration order.
func orderedAccountIndices(accounts []config.Account, reverse bool, matches, active func(config.Account) bool) []int {
	first := make([]int, 0, len(accounts))
	var rest []int
	for offset := range accounts {
		i := offset
		if reverse {
			i = len(accounts) - 1 - offset
		}
		acc := accounts[i]
		if !matches(acc) {
			continue
		}
		if active(acc) {
			first = append(first, i)
		} else {
			rest = append(rest, i)
		}
	}
	return append(first, rest...)
}

func matchesAccountQuery(acc config.Account, query string) bool {
	if query == "" {
		return true
	}
	for _, value := range []string{acc.Identifier(), acc.Name, acc.Remark, acc.Email, acc.Mobile} {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}
