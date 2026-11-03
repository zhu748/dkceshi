package client

import (
	"errors"
	"net/http"
)

type fileFetchError struct {
	status  int
	message string
}

func (e *fileFetchError) Error() string { return e.message }

func (e *fileFetchError) HTTPStatus() int {
	if e.status >= 400 && e.status <= 599 {
		return e.status
	}
	return http.StatusBadGateway
}

func isPermanentFileFetchError(err error) bool {
	if errors.Is(err, ErrUploadFileNotFound) || IsManagedUnauthorizedError(err) || IsDirectUnauthorizedError(err) || IsMutedError(err) ||
		errors.Is(err, ErrInvalidUpstreamResponse) || errors.Is(err, ErrUpstreamResponseTooLarge) {
		return true
	}
	var failure *fileFetchError
	return errors.As(err, &failure) && failure.status < 500 &&
		failure.status != http.StatusRequestTimeout && failure.status != http.StatusTooManyRequests
}
