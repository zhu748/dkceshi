package auth

import "strings"

// PinAccount prevents lower-level retries from moving resources between owners.
// Unlike TargetAccount, this is scoped to an operation and supports nesting.
func (a *RequestAuth) PinAccount() func() {
	if a == nil {
		return func() {}
	}
	a.accountPins++
	released := false
	return func() {
		if !released {
			a.accountPins--
			released = true
		}
	}
}

func (a *RequestAuth) AccountPinned() bool {
	return a != nil && (strings.TrimSpace(a.TargetAccount) != "" || a.FileAccountID != "" || a.accountPins > 0)
}
