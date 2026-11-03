package contextfile

import (
	"errors"
	"net/http"

	dsclient "ds2api/internal/deepseek/client"
)

var errContextFileReferenceLimit = errors.New("history splitting would exceed the maximum of 50 referenced files; reduce attachments or disable input_file")

func MapError(err error) (int, string) {
	var limited *dsclient.FileRateLimitError
	switch {
	case errors.Is(err, errContextFileReferenceLimit):
		return http.StatusBadRequest, err.Error()
	case dsclient.IsMutedError(err):
		return http.StatusForbidden, "Account is muted by upstream."
	case dsclient.IsBannedError(err):
		return http.StatusForbidden, "Account is banned by upstream."
	case dsclient.IsManagedUnauthorizedError(err):
		return http.StatusUnauthorized, "Account token is invalid. Please re-login the account in admin."
	case dsclient.IsDirectUnauthorizedError(err):
		return http.StatusUnauthorized, "Invalid token. If this should be a DS2API key, add it to config.keys first."
	case errors.As(err, &limited):
		return http.StatusTooManyRequests, err.Error()
	default:
		return http.StatusInternalServerError, err.Error()
	}
}
