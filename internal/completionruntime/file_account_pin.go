package completionruntime

import (
	"ds2api/internal/auth"
)

// Lower-level retries cannot move a session or an already prepared file list.
// Let the outer runtime retry rebuild generated files and the session together.
func pinCompletionAccount(a *auth.RequestAuth) func() {
	if a == nil || a.TargetAccount != "" {
		return func() {}
	}
	a.TargetAccount = a.AccountID
	return func() { a.TargetAccount = "" }
}
