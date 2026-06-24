package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/astra-go/astra"
)

// ErrorHandlerConfig configures the ErrorCatcher middleware.
type ErrorHandlerConfig struct {
	// Skipper optionally skips error handling for matched requests.
	Skipper Skipper

	// Logger is used to log errors. If nil, errors are not logged.
	Logger *slog.Logger

	// ServiceName is prepopulated into the AppError.Service field.
	ServiceName string

	// SuppressDetails when true removes error details (Details map, internal
	// messages) from the response body.  Set to false on dev/staging.
	// When nil, prod/staging mode suppresses 5xx details.
	SuppressDetails *bool
}

// DefaultErrorHandlerConfig is the default configuration used by ErrorCatcher().
var DefaultErrorHandlerConfig = ErrorHandlerConfig{
	Skipper: DefaultSkipper(),
}

// ErrorHandler returns a middleware that catches errors returned by downstream
// handlers and writes a standardized JSON error response.
//
// Standard error response format:
//
//	{
//	  "error": {
//	    "code": "USC-AUTH-1001",
//	    "message": "Token expired",
//	    "message_i18n": {"en": "...", "zh": "..."},
//	    "details": {...},
//	    "trace_id": "...",
//	    "request_id": "...",
//	    "service": "usercenter-svc",
//	    "timestamp": "2026-06-24T00:00:00Z"
//	  }
//	}
//
// This middleware should be registered early in the chain (before the router)
// to catch errors from all routes.
//
// Usage:
//
//	app.Use(middleware.ErrorHandler())
//	// or with custom config:
//	app.Use(middleware.ErrorCatcherWithConfig(middleware.ErrorHandlerConfig{
//	    ServiceName: "usercenter-svc",
//	}))
func ErrorCatcher() astra.HandlerFunc {
	return ErrorCatcherWithConfig(DefaultErrorHandlerConfig)
}

// ErrorCatcherWithConfig returns an ErrorCatcher middleware with custom config.
func ErrorCatcherWithConfig(cfg ErrorHandlerConfig) astra.HandlerFunc {
	return func(c *astra.Ctx) error {
		if shouldSkip(cfg.Skipper, c) {
			c.Next()
			return nil
		}

		// Execute the handler chain.
		if err := c.Next(); err != nil {
			// Determine if details should be suppressed.
			// Default: do not suppress. Set SuppressDetails to true for production.
			suppress := false
			if cfg.SuppressDetails != nil {
				suppress = *cfg.SuppressDetails
			}

			// Convert unknown errors to AppError (preserve the original).
			appErr := toAppError(err, suppress)

			// Enrich with context metadata.
			if appErr.TraceID == "" {
				if tid, exists := c.Get("trace_id"); exists {
					if s, ok := tid.(string); ok {
						appErr.TraceID = s
					}
				}
			}
			if appErr.RequestID == "" {
				appErr.RequestID = c.GetString("requestID")
			}
			if appErr.Service == "" && cfg.ServiceName != "" {
				appErr.Service = cfg.ServiceName
			}
			if appErr.Timestamp.IsZero() {
				appErr.Timestamp = time.Now().UTC()
			}

			// Log the error.
			if cfg.Logger != nil {
				logError(cfg.Logger, appErr)
			}

			// Build and write the standardized response.
			status := appErr.HTTPStatus
			if status <= 0 {
				if appErr.Code != "" {
					status = http.StatusBadRequest
				} else {
					status = http.StatusInternalServerError
				}
			}

			_ = c.JSON(status, appErr.ToResponse())
			return nil
		}
		return nil
	}
}

// toAppError converts any error to an *AppError for standardized handling.
// Preserves *AppError as-is, wraps *HTTPError, and creates a generic 500
// for unknown errors.
func toAppError(err error, suppress bool) *astra.AppError {
	if ae, ok := err.(*astra.AppError); ok {
		if suppress && ae.HTTPStatus >= 500 {
			// In production, replace 5xx AppError messages
			clone := *ae
			clone.Message = http.StatusText(ae.HTTPStatus)
			clone.Details = nil
			return &clone
		}
		return ae
	}

	if he, ok := err.(*astra.HTTPError); ok {
		msg := he.Message
		if _, ok := msg.(string); ok && suppress && he.Code >= 500 {
			msg = http.StatusText(he.Code)
		}
		appErr := astra.NewAppError(
			httpCodeToErrorCode(he.Code),
			he.Code,
			fmt.Sprintf("%v", msg),
		)
		if he.Err != nil {
			appErr = appErr.WithCause(he.Err)
		}
		return appErr
	}

	// Unknown error: generic 500.
	appErr := astra.NewAppError("INT-5000", http.StatusInternalServerError, "Internal Server Error")
	if !suppress {
		appErr = appErr.WithDetails("detail", err.Error())
	}
	return appErr.WithCause(err)
}

// httpCodeToErrorCode maps HTTP status codes to generic error codes.
func httpCodeToErrorCode(status int) string {
	switch {
	case status >= 500:
		return "INT-5000"
	case status == 429:
		return "RATE-429"
	case status == 404:
		return "NOTF-404"
	case status == 403:
		return "PERM-403"
	case status == 401:
		return "AUTH-401"
	case status == 400:
		return "VAL-400"
	default:
		return "INT-5000"
	}
}

// logError logs an AppError at the appropriate level based on HTTP status.
func logError(logger *slog.Logger, ae *astra.AppError) {
	attrs := []slog.Attr{
		slog.String("error_code", ae.Code),
		slog.String("error_message", ae.Message),
		slog.Int("http_status", ae.HTTPStatus),
		slog.String("trace_id", ae.TraceID),
		slog.String("request_id", ae.RequestID),
	}
	if ae.Err != nil {
		attrs = append(attrs, slog.String("cause", ae.Err.Error()))
	}
	switch {
	case ae.HTTPStatus >= 500:
		logger.LogAttrs(nil, slog.LevelError, "Server error", attrs...)
	case ae.HTTPStatus >= 400:
		logger.LogAttrs(nil, slog.LevelWarn, "Client error", attrs...)
	default:
		logger.LogAttrs(nil, slog.LevelInfo, "Error handled", attrs...)
	}
}
