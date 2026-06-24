package astra

import (
	"fmt"
	"net/http"
	"time"

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
// Standard error code format: <SERVICE>-<CATEGORY><NUMBER>
//
//	USC-AUTH-1001  — usercenter authentication error
//	ORD-VAL-2001   — order validation error
//	INT-5000       — generic internal error
//
// Categories with automatic HTTP status mapping (used by Define()):
//
//	AUTH  → 401 Unauthorized
//	VAL   → 400 Bad Request
//	NOTF  → 404 Not Found
//	CONF  → 409 Conflict
//	PERM  → 403 Forbidden
//	RATE  → 429 Too Many Requests
//	INT   → 500 Internal Server Error
//	EXT   → 502 Bad Gateway
//	TIMEOUT → 504 Gateway Timeout
//
// Defining domain errors:
//
//	// Manual creation with explicit HTTP status:
//	var ErrUserNotFound = astra.NewAppError("USC-NOTF-1001", http.StatusNotFound, "user not found")
//	var ErrEmailTaken   = astra.NewAppError("ORD-CONF-2001", http.StatusConflict, "email already registered")
//
//	// Using Define() with automatic HTTP status derivation:
//	var ErrTokenExpired = astra.Define("USC-AUTH-1001", "Token expired")
//
// Returning with extra context:
//
//	return ErrUserNotFound.WithDetails("user_id", id)
//	return ErrTokenExpired.WithTraceID(traceID).WithService("usercenter-svc")
//	return ErrInsufficientBalance.WithInternal(dbErr)
type AppError struct {
	// Code is a machine-readable, client-facing identifier, e.g. "USC-AUTH-1001".
	Code string `json:"code"`

	// HTTPStatus is the HTTP response status code.
	HTTPStatus int `json:"-"`

	// Message is a human-readable description safe to return to clients.
	Message string `json:"message"`

	// Data carries optional structured context (e.g. field names, limits).
	// Included in the response only when non-nil.
	// Deprecated: use Details instead.
	Data any `json:"data,omitempty"`

	// Details carries optional key-value context (e.g. field names, limits).
	// Included in the response only when non-nil.
	Details map[string]any `json:"details,omitempty"`

	// MessageI18n carries optional per-language translations.
	// Key is language code (e.g. "zh", "ja"), value is the localized message.
	MessageI18n map[string]string `json:"message_i18n,omitempty"`

	// Timestamp is the time the error was created (UTC).
	Timestamp time.Time `json:"timestamp,omitempty"`

	// TraceID is the distributed tracing ID (e.g. from OpenTelemetry).
	TraceID string `json:"trace_id,omitempty"`

	// RequestID is the request-scoped identifier (e.g. from RequestID middleware).
	RequestID string `json:"request_id,omitempty"`

	// Service is the name of the service that generated the error.
	Service string `json:"service,omitempty"`

	// Instance is the service instance identifier (e.g. pod name).
	Instance string `json:"instance,omitempty"`

	// Err is an internal error for logging; never sent to clients.
	Err error `json:"-"`
}

// NewAppError creates a new AppError with an auto-set Timestamp.
func NewAppError(code string, httpStatus int, message string) *AppError {
	return &AppError{
		Code:       code,
		HTTPStatus: httpStatus,
		Message:    message,
		Timestamp:  time.Now().UTC(),
	}
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

// ─── Fluent API: clone-and-set methods ────────────────────────────────────────
// All methods return a shallow clone to avoid mutating the original sentinel.

// WithData returns a shallow clone of e with Data set to data.
// Deprecated: use WithDetails instead.
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

// WithCause is an alias for WithInternal, matching GMS naming conventions.
func (e *AppError) WithCause(err error) *AppError {
	return e.WithInternal(err)
}

// WithTimestamp returns a shallow clone of e with Timestamp set.
func (e *AppError) WithTimestamp(t time.Time) *AppError {
	clone := *e
	clone.Timestamp = t
	return &clone
}

// WithTraceID returns a shallow clone of e with TraceID set.
func (e *AppError) WithTraceID(traceID string) *AppError {
	clone := *e
	clone.TraceID = traceID
	return &clone
}

// WithRequestID returns a shallow clone of e with RequestID set.
func (e *AppError) WithRequestID(requestID string) *AppError {
	clone := *e
	clone.RequestID = requestID
	return &clone
}

// WithService returns a shallow clone of e with Service set.
func (e *AppError) WithService(service string) *AppError {
	clone := *e
	clone.Service = service
	return &clone
}

// WithInstance returns a shallow clone of e with Instance set.
func (e *AppError) WithInstance(instance string) *AppError {
	clone := *e
	clone.Instance = instance
	return &clone
}

// WithDetails adds a single key-value pair to Details.
// Returns a shallow clone; original sentinel is never mutated.
func (e *AppError) WithDetails(key string, value any) *AppError {
	clone := *e
	if clone.Details == nil {
		clone.Details = make(map[string]any, 1)
	}
	clone.Details[key] = value
	return &clone
}

// WithI18n adds a single language translation.
// Returns a shallow clone; original sentinel is never mutated.
func (e *AppError) WithI18n(lang, message string) *AppError {
	clone := *e
	if clone.MessageI18n == nil {
		clone.MessageI18n = make(map[string]string, 1)
	}
	clone.MessageI18n[lang] = message
	return &clone
}

// ─── Standardized response ────────────────────────────────────────────────────

// ToResponse returns the error as a map suitable for JSON serialization.
// The map follows the standard error response protocol:
//
//	{
//	  "error": {
//	    "code": "USC-AUTH-1001",
//	    "message": "Token expired",
//	    "details": {...},
//	    "trace_id": "xxx",
//	    "request_id": "xxx",
//	    "service": "usercenter-svc",
//	    "timestamp": "2026-06-24T00:00:00Z"
//	  }
//	}
func (e *AppError) ToResponse() map[string]any {
	body := make(map[string]any, 8)
	body["code"] = e.Code
	body["message"] = e.Message

	if len(e.MessageI18n) > 0 {
		body["message_i18n"] = e.MessageI18n
	}
	if len(e.Details) > 0 {
		body["details"] = e.Details
	}
	// Deprecated Data field — include only if Details is empty
	if e.Data != nil && len(e.Details) == 0 {
		body["data"] = e.Data
	}
	if !e.Timestamp.IsZero() {
		body["timestamp"] = e.Timestamp.Format(time.RFC3339)
	}
	if e.TraceID != "" {
		body["trace_id"] = e.TraceID
	}
	if e.RequestID != "" {
		body["request_id"] = e.RequestID
	}
	if e.Service != "" {
		body["service"] = e.Service
	}
	if e.Instance != "" {
		body["instance"] = e.Instance
	}
	return map[string]any{"error": body}
}

// LocalizedMessage returns the message in the given language.
// Falls back to the default Message when the language is not available.
func (e *AppError) LocalizedMessage(lang string) string {
	if msg, ok := e.MessageI18n[lang]; ok {
		return msg
	}
	return e.Message
}

// ─── Define(): automatic HTTP status derivation ───────────────────────────────

// Define creates an AppError from a standard error code, automatically deriving
// the HTTP status from the category segment.
//
// Error code format: <SERVICE>-<CATEGORY><NUMBER>
//
//	USC-AUTH-1001  →  AUTH → 401 Unauthorized
//	ORD-VAL-2001   →  VAL  → 400 Bad Request
//	GATE-NOTF-3001 →  NOTF → 404 Not Found
//	INT-5000       →  INT  → 500 Internal Server Error
//
// Usage:
//
//	var ErrTokenExpired = astra.Define("USC-AUTH-1001", "Token expired")
//	return ErrTokenExpired.WithTraceID("abc").WithService("usercenter-svc")
func Define(code, message string) *AppError {
	httpStatus := categoryToHTTPStatus(extractCategory(code))
	return NewAppError(code, httpStatus, message)
}

// categoryToHTTPStatus maps error categories to HTTP status codes.
//
//	 AUTH  → 401 | VAL  → 400 | NOTF → 404 | CONF → 409
//	 PERM  → 403 | RATE → 429 | INT  → 500 | EXT  → 502 | TIMEOUT → 504
//
// Unknown categories fall back to 500 Internal Server Error.
func categoryToHTTPStatus(category string) int {
	switch category {
	case "AUTH":
		return http.StatusUnauthorized
	case "VAL":
		return http.StatusBadRequest
	case "NOTF":
		return http.StatusNotFound
	case "CONF":
		return http.StatusConflict
	case "PERM":
		return http.StatusForbidden
	case "RATE":
		return http.StatusTooManyRequests
	case "INT":
		return http.StatusInternalServerError
	case "EXT":
		return http.StatusBadGateway
	case "TIMEOUT":
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// extractCategory extracts the category segment from a standard error code.
//
//	"USC-AUTH-1001" → "AUTH"
//	"ORD-VAL-2001"  → "VAL"
//	"INT-5000"      → "INT"
//	"custom"        → "INT"  (falls back to Internal Server Error)
func extractCategory(code string) string {
	// Format: <SVC>-<CATEGORY>-<NUMBER> or <SVC>-<CATEGORY><NUMBER>
	// Parse by splitting on hyphens.
	parts := splitHyphen(code)
	if len(parts) >= 2 {
		return parts[len(parts)-2]
		// For "INT-5000": parts = ["INT", "5000"], return parts[-2] = "INT"
	}
	// Fallback: check if there's exactly 2 parts (no third segment)
	return "INT"
}

// splitHyphen splits a string by '-' separator.
func splitHyphen(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	return append(result, s[start:])
}

// ─── AppError utilities ──────────────────────────────────────────────────────

// IsAppError reports whether err is an *AppError.
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// AsAppError converts err to an *AppError if possible.
// Returns nil, false when err is nil or not an AppError.
func AsAppError(err error) (*AppError, bool) {
	if err == nil {
		return nil, false
	}
	ae, ok := err.(*AppError)
	return ae, ok
}

// Wrap creates an AppError wrapping an existing error.
func Wrap(err error, code, message string, httpStatus int) *AppError {
	return NewAppError(code, httpStatus, message).WithCause(err)
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
