package contract

import (
	"fmt"
	"net/http"
)

// HTTPError represents an HTTP protocol error with a status code and message.
// Middleware can return this type to signal a specific HTTP status without
// depending on the concrete astra package.
type HTTPError struct {
	Code    int
	Message any
	// Err is an optional internal error (logged but never sent to clients).
	Err error
}

// NewHTTPError creates a new HTTPError with the given status code and optional
// message.  When no message is provided the standard HTTP status text is used.
func NewHTTPError(code int, message ...any) *HTTPError {
	he := &HTTPError{Code: code}
	if len(message) > 0 {
		he.Message = message[0]
	} else {
		he.Message = http.StatusText(code)
	}
	return he
}

// Error implements the error interface.
func (he *HTTPError) Error() string {
	if he.Err != nil {
		return fmt.Sprintf("code=%d, message=%v, err=%v", he.Code, he.Message, he.Err)
	}
	return fmt.Sprintf("code=%d, message=%v", he.Code, he.Message)
}

// Unwrap returns the wrapped internal error, implementing errors.Unwrap.
func (he *HTTPError) Unwrap() error { return he.Err }

// WithInternal attaches an internal error to a clone of the HTTPError.
// The internal error is available for logging but is never forwarded to
// HTTP clients.  Returning a clone ensures that package-level sentinel
// variables (e.g. astra.ErrUnauthorized) are never mutated.
func (he *HTTPError) WithInternal(err error) *HTTPError {
	clone := *he
	clone.Err = err
	return &clone
}

// WithMessage returns a clone of the HTTPError with Message replaced.
// Use this to attach a request-specific message without mutating the
// package-level sentinel.
func (he *HTTPError) WithMessage(msg any) *HTTPError {
	clone := *he
	clone.Message = msg
	return &clone
}

// Is reports whether target is an *HTTPError with the same status code.
// This satisfies errors.Is so that a clone returned by WithInternal or
// WithMessage still matches the original sentinel:
//
//	errors.Is(ErrUnauthorized.WithInternal(err), ErrUnauthorized) // true
//
// Identity is defined by status code, not pointer address, which is the
// natural semantic for HTTP errors.
func (he *HTTPError) Is(target error) bool {
	t, ok := target.(*HTTPError)
	return ok && he.Code == t.Code
}
