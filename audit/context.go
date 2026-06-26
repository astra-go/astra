package audit

import (
	"context"
	"time"
)

type ctxKey string

const (
	ctxKeyIPAddress ctxKey = "audit_ip_address"
	ctxKeyUserAgent ctxKey = "audit_user_agent"
	ctxKeyDeviceID  ctxKey = "audit_device_id"
	ctxKeyRequestID ctxKey = "audit_request_id"
)

// WithClientMetadata returns a context carrying client metadata for audit logging.
//
//	ctx := audit.WithClientMetadata(r.Context(), ip, userAgent, deviceID, requestID)
//	audit.Log(ctx, repo, tenantID, &uin, audit.ActionLogin, audit.StatusSuccess, nil)
func WithClientMetadata(ctx context.Context, ip, userAgent, deviceID, requestID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyIPAddress, ip)
	ctx = context.WithValue(ctx, ctxKeyUserAgent, userAgent)
	ctx = context.WithValue(ctx, ctxKeyDeviceID, deviceID)
	ctx = context.WithValue(ctx, ctxKeyRequestID, requestID)
	return ctx
}

func ipFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyIPAddress).(string)
	return v
}

func userAgentFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyUserAgent).(string)
	return v
}

func deviceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyDeviceID).(string)
	return v
}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// Log writes an audit entry using metadata from the context. It falls back to
// a no-op when repo is nil, so callers can safely call it without wiring checks.
//
// Parameters:
//   - uin: user identifier (pass nil for anonymous, &uin for authenticated)
//   - action: the auditable operation type
//   - status: SUCCESS or FAILURE
//   - details: additional structured data or nil
func Log(ctx context.Context, repo Repository, tenantID int64, uin *int64, action Action, status Status, details Details) {
	if repo == nil {
		return
	}

	entry := Entry{
		TenantID:  tenantID,
		UIN:       uin,
		Action:    action,
		Status:    status,
		IPAddress: ipFromContext(ctx),
		UserAgent: userAgentFromContext(ctx),
		DeviceID:  deviceIDFromContext(ctx),
		RequestID: requestIDFromContext(ctx),
		Details:   details,
	}

	// Best-effort async logging to avoid blocking the request path.
	go func() {
		// Use a fresh background context so the write completes even if the
		// request context is cancelled.
		_ = repo.Log(context.Background(), entry)
	}()
}

// LogSync writes an audit entry synchronously.  Prefer Log() for the
// request path; use LogSync for background jobs and critical operations
// where durability is more important than latency.
func LogSync(ctx context.Context, repo Repository, tenantID int64, uin *int64, action Action, status Status, details Details) error {
	entry := Entry{
		TenantID:  tenantID,
		UIN:       uin,
		Action:    action,
		Status:    status,
		IPAddress: ipFromContext(ctx),
		UserAgent: userAgentFromContext(ctx),
		DeviceID:  deviceIDFromContext(ctx),
		RequestID: requestIDFromContext(ctx),
		Details:   details,
	}
	return repo.Log(ctx, entry)
}

// Now is a hook for testing; overridden in tests to return a fixed time.
var Now = time.Now
