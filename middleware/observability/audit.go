package observability

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/astra-go/astra"
)

// AuditEntry is the structured record written for each audited request.
//
// Request and response bodies are intentionally NOT captured — this prevents
// accidental logging of passwords, tokens, or other sensitive payloads.
type AuditEntry struct {
	Time      time.Time `json:"time"`
	ActorID   string    `json:"actor_id,omitempty"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	LatencyMS int64     `json:"latency_ms"`
	RequestID string    `json:"request_id,omitempty"`
	ClientIP  string    `json:"client_ip,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// AuditConfig configures the Audit middleware.
type AuditConfig struct {
	// GetActorID extracts the actor identifier from the request context.
	// Called after the handler chain completes so it can read values set by
	// authentication middleware upstream.
	// Default: reads the "X-User-ID" request header.
	GetActorID func(c *astra.Ctx) string

	// Logger is called once per request with the populated AuditEntry.
	// Must be safe for concurrent use. Default: writes to slog.Default().
	Logger func(entry AuditEntry)

	// Skipper returns true for requests that should not be audited.
	Skipper func(c *astra.Ctx) bool

	// AsyncBuffer is the capacity of the background write channel.
	// When > 0, the middleware sends entries to a buffered channel and returns
	// immediately; a background goroutine drains the channel and calls Logger.
	// When 0 (default), Logger is called synchronously inside the middleware.
	AsyncBuffer int
}

// Audit returns a middleware that records an AuditEntry for every request.
func Audit(cfgs ...AuditConfig) astra.HandlerFunc {
	cfg := AuditConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	if cfg.GetActorID == nil {
		cfg.GetActorID = func(c *astra.Ctx) string {
			return c.Request().Header.Get("X-User-ID")
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = defaultAuditLogger
	}

	var asyncCh chan AuditEntry
	if cfg.AsyncBuffer > 0 {
		asyncCh = make(chan AuditEntry, cfg.AsyncBuffer)
		go func() {
			for entry := range asyncCh {
				cfg.Logger(entry)
			}
		}()
	}

	write := func(entry AuditEntry) {
		if asyncCh != nil {
			select {
			case asyncCh <- entry:
			default:
			}
		} else {
			cfg.Logger(entry)
		}
	}

	return func(c *astra.Ctx) error {
		if cfg.Skipper != nil && cfg.Skipper(c) {
			c.Next()
			return nil
		}

		start := time.Now()
		c.Next()

		status := c.Writer().Status()
		latency := time.Since(start)

		var errStr string
		if status >= 500 {
			errStr = "internal server error"
		}

		entry := AuditEntry{
			Time:      start,
			ActorID:   cfg.GetActorID(c),
			Method:    c.Request().Method,
			Path:      c.Request().URL.Path,
			Status:    status,
			LatencyMS: latency.Milliseconds(),
			RequestID: c.Writer().Header().Get("X-Request-ID"),
			ClientIP:  auditClientIP(c),
			Error:     errStr,
		}

		write(entry)
		return nil
	}
}

func defaultAuditLogger(entry AuditEntry) {
	level := slog.LevelInfo
	if entry.Status >= 500 {
		level = slog.LevelError
	} else if entry.Status >= 400 {
		level = slog.LevelWarn
	}

	args := []any{
		slog.Time("time", entry.Time),
		slog.String("actor_id", entry.ActorID),
		slog.String("method", entry.Method),
		slog.String("path", entry.Path),
		slog.Int("status", entry.Status),
		slog.Int64("latency_ms", entry.LatencyMS),
	}
	if entry.RequestID != "" {
		args = append(args, slog.String("request_id", entry.RequestID))
	}
	if entry.ClientIP != "" {
		args = append(args, slog.String("client_ip", entry.ClientIP))
	}
	if entry.Error != "" {
		args = append(args, slog.String("error", entry.Error))
	}

	slog.Default().Log(context.TODO(), level, "audit", args...)
}

func auditClientIP(c *astra.Ctx) string {
	if xff := c.Request().Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	if xri := c.Request().Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := c.Request().RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[:idx]
	}
	return addr
}
