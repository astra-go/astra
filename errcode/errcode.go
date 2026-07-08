// Package errcode provides a structured error code registry, i18n key
// derivation, and common wrapper utilities for the Astra error system.
//
// It extends astra.Define() and astra.AppError with:
//   - Global error registry (for documentation, tooling, and tracing)
//   - I18n key auto-derivation from error codes
//   - Common wrappers (DB, Cache, External call)
//   - Error code introspection utilities
//
// Error code format: <SERVICE>-<CATEGORY><NUMBER>
//
//	USC-AUTH-1001  → i18n key: error.usc.auth.1001
//	ORD-VAL-2001   → i18n key: error.ord.val.2001
//
// Categories: AUTH, VAL, NOTF, CONF, PERM, RATE, INT, EXT, TIMEOUT
//
// Quick start:
//
//	// Define with automatic registry + HTTP status + i18n key
//	var ErrUserNotFound = errcode.Define("USC-NOTF-2001", "Account not found")
//
//	// Use in business logic
//	return ErrUserNotFound.WithDetails("user_id", id)
//
//	// Wrap external errors
//	return errcode.WrapDB(dbErr, "FindUser")
package errcode

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/astra-go/astra"
)

// ─── Registered error descriptor ──────────────────────────────────────────────

// ErrorDesc holds metadata for a registered error code.
type ErrorDesc struct {
	Code        string // e.g., "USC-AUTH-1001"
	Service     string // e.g., "usercenter-svc"
	Category    string // e.g., "AUTH"
	HTTPStatus  int    // Auto-derive HTTP status
	Description string // Human-readable description
	I18nKey     string // Auto-derive i18n key
}

// ─── Registry ─────────────────────────────────────────────────────────────────

// Registry is a thread-safe global store of registered error codes.
// Use Register() to add entries, and Lookup() etc. to query them.
type Registry struct {
	mu     sync.RWMutex
	errors map[string]*ErrorDesc
}

var globalRegistry = &Registry{
	errors: make(map[string]*ErrorDesc),
}

// Register adds an error description to the global registry.
// Returns the same ErrorDesc for chaining / direct assignment.
func Register(code, service, category, description string, httpStatus int) *ErrorDesc {
	e := &ErrorDesc{
		Code:        code,
		Service:     service,
		Category:    category,
		HTTPStatus:  httpStatus,
		Description: description,
		I18nKey:     I18nKey(code),
	}
	globalRegistry.mu.Lock()
	globalRegistry.errors[code] = e
	globalRegistry.mu.Unlock()
	return e
}

// MustRegister registers an error and panics on duplicate.
func MustRegister(code, service, category, description string, httpStatus int) *ErrorDesc {
	if e := Lookup(code); e != nil {
		panic(fmt.Sprintf("errcode: duplicate registration: %s", code))
	}
	return Register(code, service, category, description, httpStatus)
}

// Lookup retrieves a registered error by code.
func Lookup(code string) *ErrorDesc {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	return globalRegistry.errors[code]
}

// ListAll returns all registered errors sorted by code.
func ListAll() []*ErrorDesc {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	result := make([]*ErrorDesc, 0, len(globalRegistry.errors))
	for _, e := range globalRegistry.errors {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Code < result[j].Code
	})
	return result
}

// ListByService returns all errors for a specific service.
func ListByService(service string) []*ErrorDesc {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	result := make([]*ErrorDesc, 0)
	for _, e := range globalRegistry.errors {
		if e.Service == service {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Code < result[j].Code
	})
	return result
}

// ListByCategory returns all errors for a specific category.
func ListByCategory(category string) []*ErrorDesc {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	result := make([]*ErrorDesc, 0)
	for _, e := range globalRegistry.errors {
		if e.Category == category {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Code < result[j].Code
	})
	return result
}

// MarkdownTable generates a markdown table of all registered errors.
func MarkdownTable() string {
	var b strings.Builder
	b.WriteString("| Code | Service | Category | Description | HTTP Status | I18n Key |\n")
	b.WriteString("|------|---------|----------|-------------|-------------|----------|\n")
	for _, e := range ListAll() {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | `%s` |\n",
			e.Code, e.Service, e.Category, e.Description, e.HTTPStatus, e.I18nKey))
	}
	return b.String()
}

// Count returns the total number of registered errors.
func Count() int {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	return len(globalRegistry.errors)
}

// Reset clears all registered errors (primarily for testing).
func Reset() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.errors = make(map[string]*ErrorDesc)
}

// ─── Define: create + auto-register ───────────────────────────────────────────

// Define creates an AppError with auto-derived HTTP status and i18n key,
// and registers the error in the global registry.
//
// Error code format: <SERVICE>-<CATEGORY><NUMBER>
//
//	USC-AUTH-1001  →  AUTH → 401 | i18n key: error.usc.auth.1001
//	ORD-VAL-2001   →  VAL  → 400 | i18n key: error.ord.val.2001
//	INT-5000       →  INT  → 500 | i18n key: error.int.5000
//
// service is the human-readable service name (e.g. "usercenter-svc").
// It is NOT derived from the code prefix, because the code prefix may be
// an abbreviation and the mapping is not always 1:1.
//
// Returns an *AppError that can be used directly or extended with fluent API.
func Define(code, service, description string) *astra.AppError {
	category := CategoryFromCode(code)
	httpStatus := CategoryHTTPStatus(category)

	// Register in global registry
	Register(code, service, category, description, httpStatus)

	return astra.NewAppError(code, httpStatus, description)
}

// DefineNoRegister creates an AppError without registering in the global
// registry. Useful when you only need the AppError without tooling registration.
func DefineNoRegister(code, description string) *astra.AppError {
	category := CategoryFromCode(code)
	httpStatus := CategoryHTTPStatus(category)
	return astra.NewAppError(code, httpStatus, description)
}

// ─── I18n key derivation ──────────────────────────────────────────────────────

// I18nKey converts a standard error code to an i18n lookup key.
//
//	USC-AUTH-1001 → "error.usc.auth.1001"
//	ORD-VAL-2001  → "error.ord.val.2001"
//	INT-5000      → "error.int.5000"
func I18nKey(code string) string {
	parts := strings.SplitN(code, "-", 3)
	if len(parts) < 3 {
		// Two-part code: INT-5000 → error.int.5000
		if len(parts) == 2 {
			return fmt.Sprintf("error.%s.%s",
				strings.ToLower(parts[0]), parts[1])
		}
		return "error.internal_error"
	}
	return fmt.Sprintf("error.%s.%s.%s",
		strings.ToLower(parts[0]),
		strings.ToLower(parts[1]),
		parts[2],
	)
}

// CategoryFromCode extracts the category segment from a standard error code.
//
//	"USC-AUTH-1001" → "AUTH"
//	"ORD-VAL-2001"  → "VAL"
//	"INT-5000"      → "INT"
func CategoryFromCode(code string) string {
	parts := splitHyphen(code)
	if len(parts) >= 2 {
		// For "INT-5000": parts = ["INT", "5000"], return parts[-2] = "INT"
		return parts[len(parts)-2]
	}
	return "INT"
}

// ServicePrefixFromCode extracts the service prefix from an error code.
//
//	"USC-AUTH-1001" → "USC"
//	"ORD-VAL-2001"  → "ORD"
func ServicePrefixFromCode(code string) string {
	parts := splitHyphen(code)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// NumberFromCode extracts the numeric segment from an error code.
//
//	"USC-AUTH-1001" → "1001"
//	"INT-5000"      → "5000"
func NumberFromCode(code string) string {
	parts := splitHyphen(code)
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

// ─── Common wrappers ──────────────────────────────────────────────────────────

// WrapDBError wraps a database error as an AppError with DB-operation context.
// Creates a generic error with code prefix "INT" and message based on operation.
func WrapDBError(err error, operation string) *astra.AppError {
	return astra.NewAppError("INT-DB-0001", http.StatusInternalServerError, "database operation failed").
		WithCause(err).
		WithDetails("operation", operation).
		WithDetails("original_error", err.Error())
}

// WrapCacheError wraps a cache operation error with context.
func WrapCacheError(err error, operation string) *astra.AppError {
	return astra.NewAppError("INT-CACHE-0001", http.StatusInternalServerError, "cache operation failed").
		WithCause(err).
		WithDetails("operation", operation).
		WithDetails("original_error", err.Error())
}

// WrapExternalError wraps a third-party service call error with platform context.
func WrapExternalError(platform string, err error) *astra.AppError {
	return astra.NewAppError("EXT-0001", http.StatusBadGateway, "external service call failed").
		WithCause(err).
		WithDetails("platform", platform).
		WithDetails("original_error", err.Error())
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// CategoryHTTPStatus maps an error category to its HTTP status code.
//
//	AUTH → 401 | VAL → 400 | NOTF → 404 | CONF → 409
//	PERM → 403 | RATE → 429 | INT  → 500 | EXT  → 502 | TIMEOUT → 504
func CategoryHTTPStatus(category string) int {
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

// ─── Conventions ──────────────────────────────────────────────────────────────

// Pre-defined category constants for use in service-level Define() calls.
const (
	CatAuth    = "AUTH"    // 401 Unauthorized
	CatValid   = "VAL"     // 400 Bad Request / Validation
	CatNotF    = "NOTF"    // 404 Not Found
	CatConf    = "CONF"    // 409 Conflict
	CatPerm    = "PERM"    // 403 Forbidden
	CatRate    = "RATE"    // 429 Too Many Requests
	CatInt     = "INT"     // 500 Internal Server Error
	CatExt     = "EXT"     // 502 Bad Gateway
	CatTimeout = "TIMEOUT" // 504 Gateway Timeout
)
