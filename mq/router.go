// Package mq - HTTP route integration for the mq module.
//
// This file provides HTTP route registration for publishing messages via REST.
// It uses the HTTPRouteRegistrar interface to avoid importing the astra/ package.
package mq

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// HTTPRouteRegistrar is the interface that the HTTP router must implement.
// It mirrors the method set exposed by astra.App for route registration.
// Named HTTPRouteRegistrar to avoid confusion with astra.RouteRegistrar
// (which has a different signature: astra.HandlerFunc vs func(context, *http.Request)).
type HTTPRouteRegistrar interface {
	GET(path string, handler func(ctx context.Context, r *http.Request) error)
	POST(path string, handler func(ctx context.Context, r *http.Request) error)
	PUT(path string, handler func(ctx context.Context, r *http.Request) error)
	DELETE(path string, handler func(ctx context.Context, r *http.Request) error)
	Any(path string, handler func(ctx context.Context, r *http.Request) error)
}

// HTTPRouteOptions configures the HTTP → MQ route integration.
type HTTPRouteOptions struct {
	Producer Producer

	// EnableDelay enables ?delay= query parameter support.
	EnableDelay bool

	// EnableIdempotency enables X-Idemp-Key header for deduplication.
	EnableIdempotency bool

	// Prefix is the URL prefix for mq routes. Defaults to "/_mq".
	Prefix string

	// Compensator is the optional Redis compensator for delay/idempotency.
	Compensator *Compensator

	// IdempotencyTTL is the TTL for idempotency keys (default 24h).
	IdempotencyTTL time.Duration
}

// RegisterHTTPRoutes registers HTTP routes for publishing messages to MQ.
//
// Registered routes:
//   - POST /<prefix>/<topic> - publish a message to a topic
//
// Query parameters:
//   - delay: delay duration (e.g. "5m", "1h") when EnableDelay is true
//
// Headers:
//   - X-MQ-Key: optional partition/routing key
//   - X-MQ-Idemp-Key: idempotency key when EnableIdempotency is true
//   - X-Trace-ID: distributed tracing ID
//
// Response: 202 Accepted with body {"status":"published","topic":...,"key":...}
func RegisterHTTPRoutes(registrar HTTPRouteRegistrar, opts HTTPRouteOptions) {
	if opts.IdempotencyTTL == 0 {
		opts.IdempotencyTTL = 24 * time.Hour
	}

	producer := opts.Producer
	comp := opts.Compensator
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "/_mq"
	}

	registrar.POST(prefix+"/:topic", func(ctx context.Context, r *http.Request) error {
		topic := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if topic == "" {
			return &HTTPError{Code: 400, Message: "topic required"}
		}

		if opts.EnableIdempotency && comp != nil {
			idempKey := r.Header.Get("X-MQ-Idemp-Key")
			if idempKey != "" {
				duplicate, err := comp.CheckIdempotency(ctx, idempKey, opts.IdempotencyTTL)
				if err != nil {
					return &HTTPError{Code: 500, Message: "idempotency check failed: " + err.Error()}
				}
				if duplicate {
					return &HTTPError{Code: 409, Message: "duplicate request"}
				}
			}
		}

		msg := &Message{
			Topic:   topic,
			Payload: readBody(r),
			Key:     []byte(r.Header.Get("X-MQ-Key")),
			TraceID: r.Header.Get("X-Trace-ID"),
		}

		if opts.EnableDelay && comp != nil {
			if delayStr := r.URL.Query().Get("delay"); delayStr != "" {
				if dur, err := time.ParseDuration(delayStr); err == nil {
					msg.Delay = dur
				}
			}
		}

		if err := producer.Publish(ctx, msg); err != nil {
			return &HTTPError{Code: 503, Message: "publish failed: " + err.Error()}
		}
		return nil
	})
}

// readBody reads the request body up to 10MB.
func readBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	data := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	limit := 10 << 20 // 10MB
	for len(data) < limit {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			data = append(data, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return data
}

// HTTPError represents an HTTP-level error.
type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string { return e.Message }
