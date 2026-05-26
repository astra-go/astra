package astra

import (
	"fmt"
	"net/http"

	"github.com/astra-go/astra/contract"
)

// ─── HTTP errors ──────────────────────────────────────────────────────────────

// HTTPError is re-exported from contract for backward compatibility.
// It represents an HTTP protocol error with a status code and message.
// Use this for errors where the HTTP status code is the primary signal.
type HTTPError = contract.HTTPError

// NewHTTPError creates a new HTTPError with the given code and message.
// Re-exports contract.NewHTTPError for backward compatibility.
func NewHTTPError(code int, message ...any) *HTTPError {
	return contract.NewHTTPError(code, message...)
}

// Common HTTP errors — clone with NewHTTPError when you need a custom message.
var (
	ErrBadRequest          = NewHTTPError(http.StatusBadRequest)
	ErrUnauthorized        = NewHTTPError(http.StatusUnauthorized)
	ErrForbidden           = NewHTTPError(http.StatusForbidden)
	ErrNotFound            = NewHTTPError(http.StatusNotFound)
	ErrMethodNotAllowed    = NewHTTPError(http.StatusMethodNotAllowed)
	ErrConflict            = NewHTTPError(http.StatusConflict)
	ErrUnprocessableEntity = NewHTTPError(http.StatusUnprocessableEntity)
	ErrTooManyRequests     = NewHTTPError(http.StatusTooManyRequests)
	ErrInternalServerError = NewHTTPError(http.StatusInternalServerError)
)

// ─── Application / business errors ───────────────────────────────────────────

// AppError represents a business-layer error with a machine-readable error code.
// Use this when you need a stable, string code that clients can match against,
// in addition to an HTTP status code.
//
// Defining domain errors:
//
//	var (
//	    ErrUserNotFound  = astra.NewAppError("USER_NOT_FOUND",  http.StatusNotFound,  "user not found")
//	    ErrEmailTaken    = astra.NewAppError("EMAIL_TAKEN",     http.StatusConflict,  "email already registered")
//	    ErrInsufficientBalance = astra.NewAppError("INSUFFICIENT_BALANCE", http.StatusPaymentRequired, "insufficient balance")
//	)
//
// Returning with extra context:
//
//	return ErrUserNotFound.WithData(astra.Map{"user_id": id})
//	return ErrInsufficientBalance.WithInternal(dbErr)
type AppError struct {
	// Code is a machine-readable, client-facing identifier, e.g. "USER_NOT_FOUND".
	// Use SCREAMING_SNAKE_CASE for consistency.
	Code string `json:"code"`
	// HTTPStatus is the HTTP response status code.
	HTTPStatus int `json:"-"`
	// Message is a human-readable description safe to return to clients.
	Message string `json:"message"`
	// Data carries optional structured context (e.g. field names, limits).
	// Included in the response only when non-nil.
	Data any `json:"data,omitempty"`
	// Err is an internal error for logging; never sent to clients.
	Err error `json:"-"`
}

// NewAppError creates a new AppError.
func NewAppError(code string, httpStatus int, message string) *AppError {
	return &AppError{Code: code, HTTPStatus: httpStatus, Message: message}
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("app_error: code=%s message=%s internal=%v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("app_error: code=%s message=%s", e.Code, e.Message)
}

// Unwrap returns the internal error, implementing errors.Unwrap.
func (e *AppError) Unwrap() error { return e.Err }

// WithData returns a shallow clone of e with Data set to data.
// This is the right way to attach request-specific context without mutating
// the package-level error sentinel.
func (e *AppError) WithData(data any) *AppError {
	clone := *e
	clone.Data = data
	return &clone
}

// WithMessage returns a shallow clone of e with Message replaced.
func (e *AppError) WithMessage(msg string) *AppError {
	clone := *e
	clone.Message = msg
	return &clone
}

// WithInternal returns a shallow clone of e with an internal error attached.
// The internal error is logged by the error handler but never sent to clients.
func (e *AppError) WithInternal(err error) *AppError {
	clone := *e
	clone.Err = err
	return &clone
}

// ─── Slim-mode errors ─────────────────────────────────────────────────────────

// ErrSlimMode is returned when a feature that was disabled by NewSlim() is
// called at runtime. It signals a programming mistake: the caller registered a
// route, plugin, or lifecycle hook on an App that was created without those
// subsystems. Switch to astra.New() if the feature is required.
var ErrSlimMode = fmt.Errorf("astra: operation not available in slim mode (use astra.New())")

// ─── Validation errors ────────────────────────────────────────────────────────

// ValidationError represents a single field validation failure.
type ValidationError = contract.ValidationError

// ValidationErrors is an ordered list of field-level validation failures.
type ValidationErrors = contract.ValidationErrors

// ToValidationHTTPError wraps ValidationErrors in a 422 HTTPError.
func ToValidationHTTPError(ve ValidationErrors) *HTTPError {
	return &HTTPError{
		Code:    http.StatusUnprocessableEntity,
		Message: ve,
	}
}
