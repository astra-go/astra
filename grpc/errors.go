// errors.go — Kratos-compatible structured API errors for Astra gRPC services.
//
// Error carries four fields mirroring the Kratos error model:
//
//	Code     — HTTP status code (e.g. 404).  Used to derive the gRPC status code.
//	Reason   — Machine-readable constant (e.g. "USER_NOT_FOUND").
//	          Client code should switch on Reason, never on Message.
//	Message  — Human-readable description (may be localised or user-facing).
//	Metadata — Arbitrary key-value context attached to the error.
//
// # Wire encoding
//
// Error.GRPCStatus() serialises the error into a gRPC status whose:
//   - Code    is derived from Code via httpStatusToGRPCCode.
//   - Message is Error.Message.
//   - Details contains one errdetails.ErrorInfo entry carrying Reason and Metadata.
//
// This matches the wire format that Kratos clients already know how to decode,
// so Astra services are interoperable with Kratos clients out of the box.
//
// # Receiving errors
//
// FromError unwraps any error — whether it is already an *Error, a gRPC status
// error (with or without ErrorInfo detail), or a plain Go error — into an *Error.
// This makes it safe to call in interceptors without type-asserting manually.
package grpcserver

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Error type ───────────────────────────────────────────────────────────────

// Error is a Kratos-compatible structured API error.
// Use the constructor shortcuts (BadRequest, NotFound, …) rather than
// constructing this directly.
type Error struct {
	// Code is an HTTP status code (400, 401, 403, 404, 409, 429, 500, 503 …).
	Code int32
	// Reason is a machine-readable, UPPER_SNAKE_CASE error constant.
	// Clients should switch on Reason to handle specific error conditions.
	Reason string
	// Message is a human-readable description, suitable for logging or display.
	Message string
	// Metadata carries optional key-value context (e.g. field names, request IDs).
	Metadata map[string]string
}

// NewError creates an *Error with the given HTTP status code, reason and message.
func NewError(code int, reason, message string) *Error {
	return &Error{Code: int32(code), Reason: reason, Message: message}
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("error: code=%d reason=%s message=%s metadata=%v",
		e.Code, e.Reason, e.Message, e.Metadata)
}

// Is returns true when target is an *Error with the same Code and Reason.
// This lets callers use errors.Is(err, ErrUserNotFound) style checks.
func (e *Error) Is(target error) bool {
	var te *Error
	if errors.As(target, &te) {
		return e.Code == te.Code && e.Reason == te.Reason
	}
	return false
}

// WithMetadata returns a shallow copy of the error with Metadata set to md.
func (e *Error) WithMetadata(md map[string]string) *Error {
	clone := *e
	clone.Metadata = md
	return &clone
}

// GRPCStatus encodes the error as a gRPC status with an errdetails.ErrorInfo
// detail entry so that the Reason and Metadata survive the wire.
//
// Because *Error implements the GRPCStatus() interface, gRPC automatically
// uses this encoding when the error is returned from a handler or interceptor.
func (e *Error) GRPCStatus() *status.Status {
	s, err := status.New(httpStatusToGRPCCode(int(e.Code)), e.Message).
		WithDetails(&errdetails.ErrorInfo{
			Reason:   e.Reason,
			Metadata: e.Metadata,
		})
	if err != nil {
		// WithDetails failed (shouldn't happen in practice); fall back to a
		// plain status without the detail entry.
		return status.New(httpStatusToGRPCCode(int(e.Code)), e.Message)
	}
	return s
}

// ─── Constructor shortcuts ────────────────────────────────────────────────────

// BadRequest returns a 400 Bad Request error.
func BadRequest(reason, message string) *Error {
	return NewError(http.StatusBadRequest, reason, message)
}

// Unauthorized returns a 401 Unauthorized error.
func Unauthorized(reason, message string) *Error {
	return NewError(http.StatusUnauthorized, reason, message)
}

// Forbidden returns a 403 Forbidden error.
func Forbidden(reason, message string) *Error {
	return NewError(http.StatusForbidden, reason, message)
}

// NotFound returns a 404 Not Found error.
func NotFound(reason, message string) *Error {
	return NewError(http.StatusNotFound, reason, message)
}

// Conflict returns a 409 Conflict error.
func Conflict(reason, message string) *Error {
	return NewError(http.StatusConflict, reason, message)
}

// TooManyRequests returns a 429 Too Many Requests error.
func TooManyRequests(reason, message string) *Error {
	return NewError(http.StatusTooManyRequests, reason, message)
}

// InternalServer returns a 500 Internal Server Error.
func InternalServer(reason, message string) *Error {
	return NewError(http.StatusInternalServerError, reason, message)
}

// NotImplemented returns a 501 Not Implemented error.
func NotImplemented(reason, message string) *Error {
	return NewError(http.StatusNotImplemented, reason, message)
}

// ServiceUnavailable returns a 503 Service Unavailable error.
func ServiceUnavailable(reason, message string) *Error {
	return NewError(http.StatusServiceUnavailable, reason, message)
}

// ─── FromError ────────────────────────────────────────────────────────────────

// FromError converts any error into an *Error.
//
// Unwrapping order:
//  1. If err is already an *Error (or wraps one), it is returned as-is.
//  2. If err is a gRPC status error with an errdetails.ErrorInfo detail, the
//     Reason and Metadata are extracted and combined with the HTTP-mapped code.
//  3. If err is a gRPC status error without ErrorInfo, the code is mapped to
//     an HTTP status and the status message becomes the Message.
//  4. Any other error is wrapped as a 500 Internal Server Error.
//
// FromError never returns nil; use err == nil checks before calling.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}

	// 1. Already an *Error.
	var e *Error
	if errors.As(err, &e) {
		return e
	}

	// 2 & 3. gRPC status error.
	if gs, ok := status.FromError(err); ok {
		httpCode := grpcCodeToHTTPStatus(gs.Code()) // reuse mapping from server.go
		for _, detail := range gs.Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok {
				e := NewError(httpCode, info.Reason, gs.Message())
				e.Metadata = info.Metadata
				return e
			}
		}
		// No ErrorInfo detail — synthesise a reason from the gRPC code string.
		return NewError(httpCode, gs.Code().String(), gs.Message())
	}

	// 4. Plain Go error.
	return InternalServer("INTERNAL", err.Error())
}

// ─── Is* helpers ─────────────────────────────────────────────────────────────

// IsBadRequest reports whether err represents a 400 Bad Request.
func IsBadRequest(err error) bool { return codeOf(err) == http.StatusBadRequest }

// IsUnauthorized reports whether err represents a 401 Unauthorized.
func IsUnauthorized(err error) bool { return codeOf(err) == http.StatusUnauthorized }

// IsForbidden reports whether err represents a 403 Forbidden.
func IsForbidden(err error) bool { return codeOf(err) == http.StatusForbidden }

// IsNotFound reports whether err represents a 404 Not Found.
func IsNotFound(err error) bool { return codeOf(err) == http.StatusNotFound }

// IsConflict reports whether err represents a 409 Conflict.
func IsConflict(err error) bool { return codeOf(err) == http.StatusConflict }

// IsTooManyRequests reports whether err represents a 429 Too Many Requests.
func IsTooManyRequests(err error) bool { return codeOf(err) == http.StatusTooManyRequests }

// IsInternalServer reports whether err represents a 500 Internal Server Error.
func IsInternalServer(err error) bool { return codeOf(err) == http.StatusInternalServerError }

// IsServiceUnavailable reports whether err represents a 503 Service Unavailable.
func IsServiceUnavailable(err error) bool { return codeOf(err) == http.StatusServiceUnavailable }

// codeOf extracts the HTTP status code from any error via FromError.
func codeOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	return int(FromError(err).Code)
}

// ─── HTTP ↔ gRPC code mapping ─────────────────────────────────────────────────

// httpStatusToGRPCCode maps an HTTP status code to the closest gRPC code.
// This is the inverse of grpcCodeToHTTPStatus defined in server.go.
func httpStatusToGRPCCode(code int) codes.Code {
	switch code {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusInternalServerError:
		return codes.Internal
	case http.StatusNotImplemented:
		return codes.Unimplemented
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	case http.StatusGatewayTimeout:
		return codes.DeadlineExceeded
	default:
		if code >= 500 {
			return codes.Internal
		}
		return codes.Unknown
	}
}
