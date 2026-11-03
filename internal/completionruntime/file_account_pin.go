package completionruntime

import (
	"ds2api/internal/auth"
)

// Lower-level retries cannot move a session or an already prepared file list.
// Let the outer runtime retry rebuild generated files and the session together.
func pinCompletionAccount(a *auth.RequestAuth) func() {
	return a.PinAccount()
}
